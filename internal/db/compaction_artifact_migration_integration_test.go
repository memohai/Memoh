package db

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestCompactionArtifactMigrationPostgresPath(t *testing.T) {
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

	schema := "compaction_artifact_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE bot_history_message_compacts (
  id UUID PRIMARY KEY,
  bot_id UUID NOT NULL,
  session_id UUID,
  status TEXT NOT NULL DEFAULT 'pending',
  summary TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  usage JSONB,
  model_id UUID,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  bot_id UUID NOT NULL,
  session_id UUID,
  compact_id UUID REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL
);
`); err != nil {
		t.Fatalf("create pre-0106 compaction schema: %v", err)
	}

	legacyID := "00000000-0000-0000-0000-00000000c001"
	activeID := "00000000-0000-0000-0000-00000000d001"
	parentOneID := "00000000-0000-0000-0000-00000000d002"
	parentTwoID := "00000000-0000-0000-0000-00000000d003"
	unlinkedID := "00000000-0000-0000-0000-00000000d004"
	foreignArtifactID := "00000000-0000-0000-0000-00000000d101"
	logOnlyArtifactID := "00000000-0000-0000-0000-00000000d201"
	botID := "00000000-0000-0000-0000-00000000b001"
	foreignBotID := "00000000-0000-0000-0000-00000000b101"
	logOnlyBotID := "00000000-0000-0000-0000-00000000b201"
	sessionID := "00000000-0000-0000-0000-00000000e001"
	foreignSessionID := "00000000-0000-0000-0000-00000000e101"
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary)
VALUES ($1, $2, $3, 'ok', 'legacy summary')
`, legacyID, botID, sessionID); err != nil {
		t.Fatalf("insert pre-0106 legacy artifact: %v", err)
	}

	up0106 := readEmbeddedMigration(t, "postgres/migrations/0106_compaction_artifacts.up.sql")
	if _, err := tx.Exec(ctx, up0106); err != nil {
		t.Fatalf("apply 0106 up: %v", err)
	}
	if _, err := tx.Exec(ctx, up0106); err != nil {
		t.Fatalf("reapply 0106 up (idempotency): %v", err)
	}
	assertIndexExists(t, ctx, tx, "idx_compacts_active_session", true)
	assertForeignKeyDeleteAction(t, ctx, tx, "bot_history_message_compacts", "bot_history_message_compacts_superseded_by_fkey", "n")

	var (
		legacyVersion  int32
		legacyCoverage string
		legacyStartMs  int64
		legacyEndMs    int64
		legacyLevel    int32
		legacyParents  int32
		legacyUnmarked bool
	)
	if err := tx.QueryRow(ctx, `
SELECT artifact_version, coverage::text, anchor_start_ms, anchor_end_ms, artifact_level,
       cardinality(parent_ids), superseded_by IS NULL AND superseded_at IS NULL
FROM bot_history_message_compacts
WHERE id = $1
`, legacyID).Scan(&legacyVersion, &legacyCoverage, &legacyStartMs, &legacyEndMs, &legacyLevel, &legacyParents, &legacyUnmarked); err != nil {
		t.Fatalf("read migrated legacy artifact: %v", err)
	}
	if legacyVersion != 1 || legacyCoverage != "[]" || legacyStartMs != 0 || legacyEndMs != 0 ||
		legacyLevel != 0 || legacyParents != 0 || !legacyUnmarked {
		t.Fatalf("legacy artifact defaults = (%d, %q, %d, %d, %d, %d, %v), want (1, \"[]\", 0, 0, 0, 0, true)",
			legacyVersion, legacyCoverage, legacyStartMs, legacyEndMs, legacyLevel, legacyParents, legacyUnmarked)
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary, parent_ids)
VALUES ($1, $2, $3, 'ok', 'active', ARRAY[$4, $5]::uuid[])
`, activeID, botID, sessionID, parentOneID, parentTwoID); err != nil {
		t.Fatalf("insert active artifact: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary, superseded_by, superseded_at)
VALUES
  ($2, $4, $5, 'ok', 'parent one', $1, now()),
  ($3, $4, $5, 'ok', 'parent two', $1, now())
`, activeID, parentOneID, parentTwoID, botID, sessionID); err != nil {
		t.Fatalf("insert parent artifacts: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary)
VALUES ($1, $2, $3, 'ok', 'unlinked')
`, unlinkedID, botID, sessionID); err != nil {
		t.Fatalf("insert unlinked artifact: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary)
VALUES ($1, $2, $3, 'ok', 'foreign artifact')
	`, foreignArtifactID, foreignBotID, foreignSessionID); err != nil {
		t.Fatalf("insert foreign artifact: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, status, summary)
VALUES ($1, $2, 'ok', 'log-only artifact')
	`, logOnlyArtifactID, logOnlyBotID); err != nil {
		t.Fatalf("insert log-only artifact: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, compact_id)
VALUES
  ('00000000-0000-0000-0000-00000000a001', $1, $2, $3),
  ('00000000-0000-0000-0000-00000000a101', $4, $5, $6)
	`, botID, sessionID, legacyID, foreignBotID, foreignSessionID, foreignArtifactID); err != nil {
		t.Fatalf("insert compacted messages: %v", err)
	}

	queries := sqlc.New(tx)
	assertGeneratedLineageQueries(t, ctx, queries, botID, sessionID, []string{parentOneID, parentTwoID}, activeID)

	parsedSessionID, err := ParseUUID(sessionID)
	if err != nil {
		t.Fatalf("parse session id: %v", err)
	}
	if err := queries.DeleteMessagesBySession(ctx, parsedSessionID); err != nil {
		t.Fatalf("delete session history through generated query: %v", err)
	}
	assertRowCount(t, ctx, tx, "bot_history_messages", 1)
	assertRowCount(t, ctx, tx, "bot_history_message_compacts", 2)

	parsedForeignBotID, err := ParseUUID(foreignBotID)
	if err != nil {
		t.Fatalf("parse foreign bot id: %v", err)
	}
	if err := queries.DeleteMessagesByBot(ctx, parsedForeignBotID); err != nil {
		t.Fatalf("delete bot history through generated query: %v", err)
	}
	assertRowCount(t, ctx, tx, "bot_history_messages", 0)
	assertRowCount(t, ctx, tx, "bot_history_message_compacts", 1)

	parsedLogOnlyBotID, err := ParseUUID(logOnlyBotID)
	if err != nil {
		t.Fatalf("parse log-only bot id: %v", err)
	}
	if err := queries.DeleteCompactionLogsByBot(ctx, parsedLogOnlyBotID); err != nil {
		t.Fatalf("delete artifacts through generated query: %v", err)
	}
	assertRowCount(t, ctx, tx, "bot_history_message_compacts", 0)

	down0106 := readEmbeddedMigration(t, "postgres/migrations/0106_compaction_artifacts.down.sql")
	if _, err := tx.Exec(ctx, down0106); err != nil {
		t.Fatalf("apply 0106 down: %v", err)
	}
	assertIndexExists(t, ctx, tx, "idx_compacts_active_session", false)
	var artifactColumns int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM information_schema.columns
WHERE table_schema = $1
  AND table_name = 'bot_history_message_compacts'
  AND column_name IN (
    'artifact_version', 'coverage', 'anchor_start_ms', 'anchor_end_ms',
    'artifact_level', 'parent_ids', 'superseded_by', 'superseded_at'
  )
`, schema).Scan(&artifactColumns); err != nil {
		t.Fatalf("inspect 0106 down reversal: %v", err)
	}
	if artifactColumns != 0 {
		t.Fatalf("0106 down migration left %d artifact columns behind", artifactColumns)
	}
	assertRowCount(t, ctx, tx, "bot_history_message_compacts", 0)
}

func assertGeneratedLineageQueries(t *testing.T, ctx context.Context, queries *sqlc.Queries, botID, sessionID string, parentIDs []string, activeID string) {
	t.Helper()
	parsedBotID, err := ParseUUID(botID)
	if err != nil {
		t.Fatalf("parse bot id: %v", err)
	}
	parsedSessionID, err := ParseUUID(sessionID)
	if err != nil {
		t.Fatalf("parse session id: %v", err)
	}
	parsedActiveID, err := ParseUUID(activeID)
	if err != nil {
		t.Fatalf("parse active id: %v", err)
	}
	rows, err := queries.ListCompactionArtifactLineageBySession(ctx, parsedSessionID)
	if err != nil {
		t.Fatalf("query session lineage: %v", err)
	}
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		seen[row.ID.String()] = struct{}{}
	}
	for _, id := range append(parentIDs, activeID) {
		if _, ok := seen[id]; !ok {
			t.Fatalf("session lineage query omitted %s: %#v", id, seen)
		}
	}
	parents, err := queries.ListCompactionArtifactParentIDsBySuccessor(ctx, sqlc.ListCompactionArtifactParentIDsBySuccessorParams{
		SuccessorID: parsedActiveID,
		BotID:       parsedBotID,
		SessionID:   parsedSessionID,
	})
	if err != nil {
		t.Fatalf("query reverse lineage edges: %v", err)
	}
	gotParents := make([]string, 0, len(parents))
	for _, parent := range parents {
		gotParents = append(gotParents, parent.String())
	}
	if !reflect.DeepEqual(gotParents, parentIDs) {
		t.Fatalf("reverse lineage parents = %#v, want %#v", gotParents, parentIDs)
	}
}

func assertForeignKeyDeleteAction(t *testing.T, ctx context.Context, tx pgx.Tx, table, constraint, want string) {
	t.Helper()
	var got string
	if err := tx.QueryRow(ctx, `
SELECT confdeltype::text
FROM pg_constraint
WHERE conrelid = $1::regclass
  AND conname = $2
`, table, constraint).Scan(&got); err != nil {
		t.Fatalf("read %s delete action: %v", constraint, err)
	}
	if got != want {
		t.Fatalf("%s delete action = %q, want %q", constraint, got, want)
	}
}

func assertIndexExists(t *testing.T, ctx context.Context, tx pgx.Tx, index string, want bool) {
	t.Helper()
	var got bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", index).Scan(&got); err != nil {
		t.Fatalf("inspect index %s: %v", index, err)
	}
	if got != want {
		t.Fatalf("index %s existence = %v, want %v", index, got, want)
	}
}

func assertRowCount(t *testing.T, ctx context.Context, tx pgx.Tx, table string, want int) {
	t.Helper()
	var got int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s row count = %d, want %d", table, got, want)
	}
}
