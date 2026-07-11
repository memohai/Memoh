package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompactionTerminalStatusMigrationPostgresPath(t *testing.T) {
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
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "compaction_terminal_status_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ok', 'error'))
);
`); err != nil {
		t.Fatalf("create pre-0107 schema: %v", err)
	}
	up := readEmbeddedMigration(t, "postgres/migrations/0107_compaction_terminal_status.up.sql")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply 0107 up: %v", err)
	}
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("reapply 0107 up: %v", err)
	}

	pendingLogID := uuid.NewString()
	pendingSuccessLogID := uuid.NewString()
	okLogID := uuid.NewString()
	errorLogID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, status)
VALUES ($1, 'pending'), ($2, 'pending'), ($3, 'ok'), ($4, 'error')
`, pendingLogID, pendingSuccessLogID, okLogID, errorLogID); err != nil {
		t.Fatalf("insert terminal status fixtures: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_history_message_compacts SET status = 'ok' WHERE id = $1`, pendingSuccessLogID); err != nil {
		t.Fatalf("complete pending attempt successfully: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_history_message_compacts SET status = 'error' WHERE id = $1`, pendingLogID); err != nil {
		t.Fatalf("terminalize pending attempt: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_history_message_compacts SET status = 'error' WHERE id = $1`, errorLogID); err != nil {
		t.Fatalf("repeat same terminal status: %v", err)
	}
	assertPostgresStatementFails(t, ctx, tx, "revive_error_attempt", `
UPDATE bot_history_message_compacts SET status = 'ok' WHERE id = $1
`, pendingLogID)
	assertPostgresStatementFails(t, ctx, tx, "rewrite_ok_attempt", `
UPDATE bot_history_message_compacts SET status = 'error' WHERE id = $1
`, okLogID)

	down := readEmbeddedMigration(t, "postgres/migrations/0107_compaction_terminal_status.down.sql")
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("apply 0107 down: %v", err)
	}
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("reapply 0107 down: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_history_message_compacts SET status = 'ok' WHERE id = $1`, pendingLogID); err != nil {
		t.Fatalf("legacy terminal rewrite remained blocked after down: %v", err)
	}
}
