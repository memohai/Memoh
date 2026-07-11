package db

import (
	"context"
	"fmt"
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
  CONSTRAINT bot_history_message_compacts_superseded_by_fkey
    FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL
);
CREATE INDEX idx_compacts_active_session
  ON bot_history_message_compacts(session_id, anchor_start_ms, started_at)
  WHERE status = 'ok' AND superseded_at IS NULL;
`); err != nil {
		t.Fatalf("create old 0106 compaction schema: %v", err)
	}
	assertForeignKeyDeleteAction(t, ctx, tx, "bot_history_message_compacts", "bot_history_message_compacts_superseded_by_fkey", "n")
	assertIndexExists(t, ctx, tx, "idx_compacts_active_session", true)
	assertIndexExists(t, ctx, tx, "idx_compacts_session_lineage", false)
	assertConstraintExists(t, ctx, tx, "bot_history_message_compacts", "compacts_supersession_markers_check", false)
	assertConstraintExists(t, ctx, tx, "bot_history_message_compacts", "compacts_not_self_superseded_check", false)
	activeID := "00000000-0000-0000-0000-00000000d001"
	parentOneID := "00000000-0000-0000-0000-00000000d002"
	parentTwoID := "00000000-0000-0000-0000-00000000d003"
	unlinkedID := "00000000-0000-0000-0000-00000000d004"
	missingID := "00000000-0000-0000-0000-00000000d099"
	foreignArtifactID := "00000000-0000-0000-0000-00000000d101"
	botID := "00000000-0000-0000-0000-00000000b001"
	foreignBotID := "00000000-0000-0000-0000-00000000b101"
	sessionID := "00000000-0000-0000-0000-00000000e001"
	foreignSessionID := "00000000-0000-0000-0000-00000000e101"
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

	up0107 := readEmbeddedMigration(t, "postgres/migrations/0107_compaction_artifact_parent_edges.up.sql")
	if _, err := tx.Exec(ctx, up0107); err != nil {
		t.Fatalf("apply 0107 up after 0106: %v", err)
	}
	queries := sqlc.New(tx)
	parents := []string{parentOneID, parentTwoID}
	assertGeneratedLineageQueries(t, ctx, queries, botID, sessionID, parents, activeID)
	assertGeneratedParentEdges(t, ctx, queries, activeID, []expectedParentEdge{
		{parentID: parentOneID, ordinal: 0},
		{parentID: parentTwoID, ordinal: 1},
	})
	assertIndexExists(t, ctx, tx, "idx_compacts_session_lineage", true)
	assertForeignKeyDeleteAction(t, ctx, tx, "bot_history_message_compacts", "bot_history_message_compacts_superseded_by_fkey", "a")
	assertConstraintExists(t, ctx, tx, "bot_history_message_compacts", "compacts_supersession_markers_check", true)
	assertConstraintExists(t, ctx, tx, "bot_history_message_compacts", "compacts_not_self_superseded_check", true)
	assertForeignKeyDeleteAction(t, ctx, tx, "bot_history_message_compact_parent_edges", "compaction_artifact_parent_edges_artifact_fkey", "c")
	assertForeignKeyDeleteAction(t, ctx, tx, "bot_history_message_compact_parent_edges", "compaction_artifact_parent_edges_parent_fkey", "a")

	if _, err := tx.Exec(ctx, `
UPDATE bot_history_message_compacts
SET parent_ids = ARRAY[$2, $1]::uuid[]
WHERE id = $3
`, parentOneID, parentTwoID, activeID); err != nil {
		t.Fatalf("reorder parent ids through sync trigger: %v", err)
	}
	assertGeneratedParentEdges(t, ctx, queries, activeID, []expectedParentEdge{
		{parentID: parentTwoID, ordinal: 0},
		{parentID: parentOneID, ordinal: 1},
	})
	if _, err := tx.Exec(ctx, up0107); err != nil {
		t.Fatalf("reapply 0107 up: %v", err)
	}
	assertGeneratedParentEdges(t, ctx, queries, activeID, []expectedParentEdge{
		{parentID: parentTwoID, ordinal: 0},
		{parentID: parentOneID, ordinal: 1},
	})

	assertPostgresStatementFails(t, ctx, tx, "marker_check", `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary, superseded_by)
VALUES ('00000000-0000-0000-0000-00000000d005', $2, $3, 'ok', 'invalid', $1)
`, activeID, botID, sessionID)
	assertPostgresStatementFails(t, ctx, tx, "self_superseded", `
INSERT INTO bot_history_message_compacts (
  id, bot_id, session_id, status, summary, superseded_by, superseded_at
)
VALUES (
  '00000000-0000-0000-0000-00000000d006', $1, $2, 'ok', 'invalid',
  '00000000-0000-0000-0000-00000000d006', now()
)
`, botID, sessionID)
	assertPostgresStatementFails(t, ctx, tx, "duplicate_parent", `
INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
VALUES ($1, $2, 2)
`, activeID, parentOneID)
	assertPostgresStatementFails(t, ctx, tx, "duplicate_ordinal", `
INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
VALUES ($1, $2, 0)
`, activeID, unlinkedID)
	assertPostgresStatementFails(t, ctx, tx, "negative_ordinal", `
INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
VALUES ($1, $2, -1)
`, unlinkedID, parentOneID)
	assertPostgresStatementFails(t, ctx, tx, "self_parent", `
INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
VALUES ($1, $1, 0)
`, unlinkedID)
	assertPostgresStatementFails(t, ctx, tx, "missing_parent", `
INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
VALUES ($1, $2, 0)
`, unlinkedID, missingID)
	assertPostgresStatementFails(t, ctx, tx, "missing_artifact", `
INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
VALUES ($1, $2, 0)
`, missingID, unlinkedID)
	assertPostgresStatementFails(t, ctx, tx, "parent_delete", `DELETE FROM bot_history_message_compacts WHERE id = $1`, parentOneID)
	assertPostgresStatementFails(t, ctx, tx, "successor_delete", `DELETE FROM bot_history_message_compacts WHERE id = $1`, activeID)
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary, parent_ids)
VALUES ($1, $2, $3, 'ok', 'foreign artifact', ARRAY[$4]::uuid[])
`, foreignArtifactID, foreignBotID, foreignSessionID, parentOneID); err != nil {
		t.Fatalf("insert foreign artifact with inbound parent edge: %v", err)
	}
	assertGeneratedParentEdges(t, ctx, queries, foreignArtifactID, []expectedParentEdge{{parentID: parentOneID, ordinal: 0}})

	parsedBotID, err := ParseUUID(botID)
	if err != nil {
		t.Fatalf("parse bot id for whole-lineage delete: %v", err)
	}
	assertPostgresOperationFails(t, ctx, tx, "cross_owner_lineage_delete", func() error {
		return queries.DeleteCompactionLogsByBot(ctx, parsedBotID)
	})
	if _, err := tx.Exec(ctx, `
UPDATE bot_history_message_compacts
SET parent_ids = '{}'::uuid[]
WHERE id = $1
`, foreignArtifactID); err != nil {
		t.Fatalf("detach foreign parent edge: %v", err)
	}
	if err := queries.DeleteCompactionLogsByBot(ctx, parsedBotID); err != nil {
		t.Fatalf("delete whole lineage through generated query: %v", err)
	}
	assertRowCount(t, ctx, tx, "bot_history_message_compacts", 1)
	assertRowCount(t, ctx, tx, "bot_history_message_compact_parent_edges", 0)

	down0107 := readEmbeddedMigration(t, "postgres/migrations/0107_compaction_artifact_parent_edges.down.sql")
	if _, err := tx.Exec(ctx, down0107); err != nil {
		t.Fatalf("apply 0107 down: %v", err)
	}
	var reversed bool
	if err := tx.QueryRow(ctx, `
SELECT to_regclass('bot_history_message_compact_parent_edges') IS NULL
  AND to_regprocedure('sync_compaction_artifact_parent_edges()') IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM pg_trigger
    WHERE tgrelid = 'bot_history_message_compacts'::regclass
      AND tgname = 'compaction_artifact_parent_edges_sync'
  )
`).Scan(&reversed); err != nil {
		t.Fatalf("inspect 0107 down reversal: %v", err)
	}
	if !reversed {
		t.Fatal("0107 down migration left normalized parent-edge objects behind")
	}
	assertIndexExists(t, ctx, tx, "idx_compacts_session_lineage", false)
	assertIndexExists(t, ctx, tx, "idx_compacts_active_session", true)
	assertForeignKeyDeleteAction(t, ctx, tx, "bot_history_message_compacts", "bot_history_message_compacts_superseded_by_fkey", "n")
	assertConstraintExists(t, ctx, tx, "bot_history_message_compacts", "compacts_supersession_markers_check", false)
	assertConstraintExists(t, ctx, tx, "bot_history_message_compacts", "compacts_not_self_superseded_check", false)
	var parentIDsExists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = $1
    AND table_name = 'bot_history_message_compacts'
    AND column_name = 'parent_ids'
)
`, schema).Scan(&parentIDsExists); err != nil {
		t.Fatalf("inspect retained 0106 schema: %v", err)
	}
	if !parentIDsExists {
		t.Fatal("0107 down migration removed the 0106 parent_ids source of truth")
	}
}

type expectedParentEdge struct {
	parentID string
	ordinal  int32
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

func assertGeneratedParentEdges(t *testing.T, ctx context.Context, queries *sqlc.Queries, artifactID string, want []expectedParentEdge) {
	t.Helper()
	parsedArtifactID, err := ParseUUID(artifactID)
	if err != nil {
		t.Fatalf("parse artifact id: %v", err)
	}
	edges, err := queries.ListCompactionArtifactParentEdges(ctx, parsedArtifactID)
	if err != nil {
		t.Fatalf("query normalized parent edges: %v", err)
	}
	got := make([]expectedParentEdge, 0, len(edges))
	for _, edge := range edges {
		got = append(got, expectedParentEdge{parentID: edge.ParentID.String(), ordinal: edge.Ordinal})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized parent edges = %#v, want %#v", got, want)
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

func assertConstraintExists(t *testing.T, ctx context.Context, tx pgx.Tx, table, constraint string, want bool) {
	t.Helper()
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_constraint
  WHERE conrelid = $1::regclass
    AND conname = $2
)
`, table, constraint).Scan(&exists); err != nil {
		t.Fatalf("inspect constraint %s: %v", constraint, err)
	}
	if exists != want {
		t.Fatalf("constraint %s existence = %v, want %v", constraint, exists, want)
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

func assertPostgresStatementFails(t *testing.T, ctx context.Context, tx pgx.Tx, name, statement string, args ...any) {
	t.Helper()
	assertPostgresOperationFails(t, ctx, tx, name, func() error {
		_, err := tx.Exec(ctx, statement, args...)
		return err
	})
}

func assertPostgresOperationFails(t *testing.T, ctx context.Context, tx pgx.Tx, name string, operation func() error) {
	t.Helper()
	savepoint := pgx.Identifier{"savepoint_" + name}.Sanitize()
	if _, err := tx.Exec(ctx, "SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("create %s savepoint: %v", name, err)
	}
	if err := operation(); err == nil {
		t.Fatalf("%s unexpectedly succeeded", name)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", savepoint)); err != nil {
		t.Fatalf("rollback %s savepoint: %v", name, err)
	}
}
