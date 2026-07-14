package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// TestListUncompactedMessagesReclaimEligibility exercises the real SQL
// eligibility predicate against PostgreSQL: rows stay selectable unless their
// compact log holds a usable summary (status ok AND non-empty), matching the
// read path's substitution predicate, and passive_sync rows never enter the
// candidate set.
func TestListUncompactedMessagesReclaimEligibility(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "uncompacted_query_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema+", public"); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	if _, err := tx.Exec(ctx, baseline); err != nil {
		t.Fatalf("apply 0001 baseline: %v", err)
	}

	userID := uuid.NewString()
	botID := uuid.NewString()
	sessionID := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO users (id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bots (id, owner_user_id, type, name) VALUES ($1, $2, 'personal', 'reclaim-test')`, botID, userID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_sessions (id, bot_id) VALUES ($1, $2)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	logs := map[string]string{
		"usable":     uuid.NewString(),
		"error":      uuid.NewString(),
		"pending":    uuid.NewString(),
		"poison":     uuid.NewString(),
		"whitespace": uuid.NewString(),
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary, message_count) VALUES
  ($1, $6, $7, 'ok', 'a usable summary', 1),
  ($2, $6, $7, 'error', '', 0),
  ($3, $6, $7, 'pending', '', 0),
  ($4, $6, $7, 'ok', '', 1),
  ($5, $6, $7, 'ok', E'  \n\t', 1)
`, logs["usable"], logs["error"], logs["pending"], logs["poison"], logs["whitespace"], botID, sessionID); err != nil {
		t.Fatalf("insert compact logs: %v", err)
	}

	type fixture struct {
		name      string
		compactID string
		metadata  string
		eligible  bool
	}
	fixtures := []fixture{
		{name: "plain", eligible: true},
		{name: "covered by usable summary", compactID: logs["usable"], eligible: false},
		{name: "log failed", compactID: logs["error"], eligible: true},
		{name: "log never completed", compactID: logs["pending"], eligible: true},
		{name: "legacy ok with empty summary", compactID: logs["poison"], eligible: true},
		{name: "legacy ok with whitespace-only summary", compactID: logs["whitespace"], eligible: true},
		{name: "passive sync", metadata: `{"trigger_mode":"passive_sync"}`, eligible: false},
	}
	wantEligible := make(map[string]string)
	// The fixtures model rows that predate the claim-guard triggers (an
	// upgraded database), so they are inserted with triggers disabled the
	// way replication/restore paths write historical data.
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role = replica"); err != nil {
		t.Fatalf("disable triggers for legacy fixtures: %v", err)
	}
	for i, f := range fixtures {
		id := uuid.NewString()
		metadata := f.metadata
		if metadata == "" {
			metadata = "{}"
		}
		var compactID any
		if f.compactID != "" {
			compactID = f.compactID
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages
  (id, bot_id, session_id, role, content, metadata, compact_id, turn_visible, turn_id, turn_position, turn_message_seq, created_at)
VALUES
  ($1, $2, $3, 'user', '[{"type":"text","text":"m"}]', $4, $5, true, $6, $7, 0, now() + make_interval(secs => $8))
`, id, botID, sessionID, metadata, compactID, uuid.NewString(), i, i); err != nil {
			t.Fatalf("insert message %q: %v", f.name, err)
		}
		if f.eligible {
			wantEligible[id] = f.name
		}
	}

	// Mirror the 0109 upgrade backfill: legacy claims against a successful
	// artifact arrive finalized on an upgraded database.
	if _, err := tx.Exec(ctx, `
UPDATE bot_history_messages message
SET compact_claim_finalized = true
FROM bot_history_message_compacts compact
WHERE message.compact_id = compact.id
  AND compact.status = 'ok'
  AND message.compact_claim_finalized = false
`); err != nil {
		t.Fatalf("apply legacy claim backfill: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role = origin"); err != nil {
		t.Fatalf("re-enable triggers: %v", err)
	}

	var sessionUUID pgtype.UUID
	if err := sessionUUID.Scan(sessionID); err != nil {
		t.Fatalf("scan session uuid: %v", err)
	}
	rows, err := sqlc.New(tx).ListUncompactedMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("ListUncompactedMessagesBySession: %v", err)
	}

	got := make(map[string]bool)
	for i, row := range rows {
		id := uuid.UUID(row.ID.Bytes).String()
		got[id] = true
		if i > 0 && row.CreatedAt.Time.Before(rows[i-1].CreatedAt.Time) {
			t.Fatalf("rows not ordered by created_at: %v after %v", row.CreatedAt.Time, rows[i-1].CreatedAt.Time)
		}
	}
	for id, name := range wantEligible {
		if !got[id] {
			t.Errorf("row %q missing from candidate set", name)
		}
	}
	if len(got) != len(wantEligible) {
		t.Errorf("candidate set size = %d, want %d (an excluded row leaked in)", len(got), len(wantEligible))
	}
}
