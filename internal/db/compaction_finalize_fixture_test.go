package db

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func openCompactionFinalizeTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := "compaction_finalize_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse postgres config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `
CREATE TABLE models (
  id UUID PRIMARY KEY
);
CREATE TABLE bot_history_message_compacts (
  id UUID PRIMARY KEY,
  bot_id UUID NOT NULL,
  session_id UUID,
  status TEXT NOT NULL DEFAULT 'pending',
  summary TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  usage JSONB,
  model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  artifact_version INTEGER NOT NULL DEFAULT 1,
  coverage JSONB NOT NULL DEFAULT '[]'::jsonb,
  anchor_start_ms BIGINT NOT NULL DEFAULT 0,
  anchor_end_ms BIGINT NOT NULL DEFAULT 0,
  artifact_level INTEGER NOT NULL DEFAULT 0,
  parent_ids UUID[] NOT NULL DEFAULT '{}'::uuid[],
  superseded_by UUID,
  superseded_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  CONSTRAINT reject_summary CHECK (summary <> 'reject')
);
CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  bot_id UUID NOT NULL,
  session_id UUID,
  role TEXT NOT NULL DEFAULT 'user',
  content JSONB NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}',
  compact_id UUID REFERENCES bot_history_message_compacts(id),
  turn_id UUID,
  turn_position BIGINT,
  turn_message_seq BIGINT,
  turn_visible BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT to_timestamp(0.001),
  source_revision BIGINT NOT NULL DEFAULT 1
);
`); err != nil {
		t.Fatalf("create finalize fixture: %v", err)
	}
	for _, migration := range []string{
		"postgres/migrations/0108_compaction_terminal_status.up.sql",
		"postgres/migrations/0109_compaction_claim_finalization.up.sql",
	} {
		if _, err := pool.Exec(ctx, readEmbeddedMigration(t, migration)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	if _, err := pool.Exec(ctx, `
CREATE OR REPLACE FUNCTION bump_history_message_source_revision()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF (
    to_jsonb(NEW) - ARRAY[
      'compact_id',
      'compact_claim_finalized',
      'compact_claim_invalidated',
      'source_revision'
    ]
  ) IS DISTINCT FROM (
    to_jsonb(OLD) - ARRAY[
      'compact_id',
      'compact_claim_finalized',
      'compact_claim_invalidated',
      'source_revision'
    ]
  ) OR NEW.source_revision IS DISTINCT FROM OLD.source_revision THEN
    NEW.source_revision := OLD.source_revision + 1;
  ELSE
    NEW.source_revision := OLD.source_revision;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER history_message_source_revision_bump
BEFORE UPDATE ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION bump_history_message_source_revision();
`); err != nil {
		t.Fatalf("install source revision fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0113_compaction_claim_invalidation.up.sql")); err != nil {
		t.Fatalf("apply claim invalidation migration: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0115_compaction_validation_provenance.up.sql")); err != nil {
		t.Fatalf("apply validation provenance migration: %v", err)
	}
	return pool
}

func finalizeParams(
	logID, botID, sessionID pgtype.UUID,
	messageIDs []pgtype.UUID,
	versions []string,
	summary string,
	expected ...[]string,
) sqlc.FinalizeCompactionArtifactParams {
	expectedCompactIDs := make([]string, len(messageIDs))
	if len(expected) > 0 {
		expectedCompactIDs = expected[0]
	}
	coverage := make([]map[string]any, len(messageIDs))
	for index, messageID := range messageIDs {
		coverage[index] = map[string]any{
			"ref": map[string]any{
				"namespace":    "bot_history_message",
				"id":           messageID.String(),
				"version":      1,
				"hash_algo":    "sha256",
				"content_hash": strings.Repeat("a", 64),
				"hash_scope":   "source_payload",
				"schema":       "context_ref",
				"durability":   "durable",
			},
			"created_at_ms": 1,
		}
	}
	encodedCoverage, _ := json.Marshal(coverage)
	return sqlc.FinalizeCompactionArtifactParams{
		CompactID:          logID,
		BotID:              botID,
		SessionID:          sessionID,
		MessageIds:         messageIDs,
		SourceVersions:     versions,
		ExpectedCompactIds: expectedCompactIDs,
		Summary:            summary,
		Usage:              []byte(`{"total_tokens":120}`),
		Coverage:           encodedCoverage,
		AnchorStartMs:      1,
		AnchorEndMs:        1,
	}
}

func insertFinalizeMessages(t *testing.T, ctx context.Context, pool *pgxpool.Pool, botID, sessionID pgtype.UUID, messageIDs []pgtype.UUID) {
	t.Helper()
	for index, messageID := range messageIDs {
		if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, content, turn_id, turn_position, turn_message_seq)
VALUES ($1, $2, $3, jsonb_build_object('index', $4::integer), $5, $4, 1)
`, messageID, botID, sessionID, index+1, testUUID()); err != nil {
			t.Fatalf("insert source message: %v", err)
		}
	}
}

func insertFinalizeLogs(t *testing.T, ctx context.Context, pool *pgxpool.Pool, botID, sessionID pgtype.UUID, logIDs []pgtype.UUID) {
	t.Helper()
	for _, logID := range logIDs {
		if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id) VALUES ($1, $2, $3)
`, logID, botID, sessionID); err != nil {
			t.Fatalf("insert compaction log: %v", err)
		}
	}
}

func readMessageVersions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageIDs []pgtype.UUID) []string {
	t.Helper()
	versions := make([]string, len(messageIDs))
	for index, messageID := range messageIDs {
		if err := pool.QueryRow(ctx, `
SELECT COALESCE(to_jsonb(message)->>'source_revision', message.xmin::text)
FROM bot_history_messages message
WHERE message.id = $1
`, messageID).Scan(&versions[index]); err != nil {
			t.Fatalf("read source version: %v", err)
		}
	}
	return versions
}

func assertClaimedBy(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageIDs []pgtype.UUID, compactID pgtype.UUID) {
	t.Helper()
	for _, messageID := range messageIDs {
		var got pgtype.UUID
		var finalized bool
		if err := pool.QueryRow(ctx, `
SELECT compact_id, compact_claim_finalized
FROM bot_history_messages
WHERE id = $1
`, messageID).Scan(&got, &finalized); err != nil {
			t.Fatalf("read source claim: %v", err)
		}
		if got != compactID {
			t.Fatalf("message %s compact_id = %s, want %s", messageID, got, compactID)
		}
		if !finalized {
			t.Fatalf("message %s claim for %s is not finalized", messageID, compactID)
		}
	}
}

func assertSingleSuccessfulLog(t *testing.T, ctx context.Context, pool *pgxpool.Pool, logIDs []pgtype.UUID, winner pgtype.UUID) {
	t.Helper()
	for _, logID := range logIDs {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM bot_history_message_compacts WHERE id = $1`, logID).Scan(&status); err != nil {
			t.Fatalf("read compaction log: %v", err)
		}
		want := "error"
		if logID == winner {
			want = "ok"
		}
		if status != want {
			t.Fatalf("log %s status = %q, want %q", logID, status, want)
		}
	}
}

func assertStatusAndUnclaimed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, logID pgtype.UUID, messageIDs []pgtype.UUID, wantStatus string) {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM bot_history_message_compacts WHERE id = $1`, logID).Scan(&status); err != nil {
		t.Fatalf("read compaction log: %v", err)
	}
	if status != wantStatus {
		t.Fatalf("log status = %q, want %q", status, wantStatus)
	}
	for _, messageID := range messageIDs {
		var compactID pgtype.UUID
		var finalized bool
		if err := pool.QueryRow(ctx, `
SELECT compact_id, compact_claim_finalized
FROM bot_history_messages
WHERE id = $1
`, messageID).Scan(&compactID, &finalized); err != nil {
			t.Fatalf("read source claim: %v", err)
		}
		if compactID.Valid {
			t.Fatalf("message %s was partially claimed by %s", messageID, compactID)
		}
		if finalized {
			t.Fatalf("unclaimed message %s retained a finalized claim marker", messageID)
		}
	}
}

func testUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}
