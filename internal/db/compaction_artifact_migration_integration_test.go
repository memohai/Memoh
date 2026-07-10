package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
  session_id UUID,
  status TEXT NOT NULL DEFAULT 'pending',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  superseded_by UUID
);
ALTER TABLE bot_history_message_compacts
  ADD CONSTRAINT bot_history_message_compacts_superseded_by_fkey
  FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL;
`); err != nil {
		t.Fatalf("create legacy compaction table: %v", err)
	}

	up := readEmbeddedMigration(t, "postgres/migrations/0105_compaction_artifacts.up.sql")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply 0105 up: %v", err)
	}
	activeID := "00000000-0000-0000-0000-00000000d001"
	parentID := "00000000-0000-0000-0000-00000000d002"
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id, status) VALUES ($1, 'ok')`, activeID); err != nil {
		t.Fatalf("insert active artifact: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, status, superseded_by, superseded_at)
VALUES ($2, 'ok', $1, now())
`, activeID, parentID); err != nil {
		t.Fatalf("insert parent artifact: %v", err)
	}
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("reapply 0105 up: %v", err)
	}

	var deleteAction string
	if err := tx.QueryRow(ctx, `
SELECT confdeltype::text
FROM pg_constraint
WHERE conrelid = 'bot_history_message_compacts'::regclass
  AND conname = 'bot_history_message_compacts_superseded_by_fkey'
`).Scan(&deleteAction); err != nil {
		t.Fatalf("read lineage foreign key: %v", err)
	}
	if deleteAction != "a" {
		t.Fatalf("lineage foreign key delete action = %q, want NO ACTION", deleteAction)
	}
	assertPostgresStatementFails(t, ctx, tx, "marker_check", `
INSERT INTO bot_history_message_compacts (id, status, superseded_by)
VALUES ('00000000-0000-0000-0000-00000000d003', 'ok', $1)
`, activeID)
	assertPostgresStatementFails(t, ctx, tx, "lineage_delete", `DELETE FROM bot_history_message_compacts WHERE id = $1`, activeID)

	var indexName string
	if err := tx.QueryRow(ctx, `SELECT to_regclass('idx_compacts_session_lineage')::text`).Scan(&indexName); err != nil {
		t.Fatalf("read lineage index: %v", err)
	}
	if indexName == "" {
		t.Fatal("session lineage index was not created")
	}

	down := readEmbeddedMigration(t, "postgres/migrations/0105_compaction_artifacts.down.sql")
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("apply 0105 down: %v", err)
	}
	var coverageExists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.columns
  WHERE table_schema = $1
    AND table_name = 'bot_history_message_compacts'
    AND column_name = 'coverage'
)
`, schema).Scan(&coverageExists); err != nil {
		t.Fatalf("inspect down migration: %v", err)
	}
	if coverageExists {
		t.Fatal("0105 down migration left artifact columns behind")
	}
}

func assertPostgresStatementFails(t *testing.T, ctx context.Context, tx pgx.Tx, name, statement string, args ...any) {
	t.Helper()
	savepoint := pgx.Identifier{"savepoint_" + name}.Sanitize()
	if _, err := tx.Exec(ctx, "SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("create %s savepoint: %v", name, err)
	}
	if _, err := tx.Exec(ctx, statement, args...); err == nil {
		t.Fatalf("%s unexpectedly succeeded", name)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", savepoint)); err != nil {
		t.Fatalf("rollback %s savepoint: %v", name, err)
	}
}
