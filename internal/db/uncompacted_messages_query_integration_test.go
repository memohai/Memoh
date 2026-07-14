//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// TestListUncompactedMessagesReclaimEligibility exercises the real SQL
// eligibility predicate against PostgreSQL: rows stay selectable unless their
// compact log holds a usable summary (status ok AND non-empty), matching the
// read path's substitution predicate, and passive_sync rows never enter the
// candidate set.
func TestListUncompactedMessagesReclaimEligibility(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.team_id', '00000000-0000-0000-0000-000000000001', true)"); err != nil {
		t.Fatalf("bind singleton team: %v", err)
	}

	userID := uuid.NewString()
	botID := uuid.NewString()
	sessionID := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO users (id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bots (id, owner_user_id, name) VALUES ($1, $2, 'reclaim-test')`, botID, userID); err != nil {
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
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary) VALUES
  ($1, $6, $7, 'ok', 'a usable summary'),
  ($2, $6, $7, 'error', ''),
  ($3, $6, $7, 'pending', ''),
  ($4, $6, $7, 'ok', ''),
  ($5, $6, $7, 'ok', E'  \n\t')
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
