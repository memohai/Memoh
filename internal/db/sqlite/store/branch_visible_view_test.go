package store

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

type testSQLExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func createBranchVisibleMessagesView(t *testing.T, conn testSQLExecutor) {
	t.Helper()
	migration, err := os.ReadFile("../../../../db/sqlite/migrations/0001_init.up.sql")
	if err != nil {
		t.Fatalf("read sqlite baseline migration: %v", err)
	}
	const viewStart = "CREATE VIEW IF NOT EXISTS bot_branch_visible_messages AS"
	sqlText := string(migration)
	start := strings.Index(sqlText, viewStart)
	if start < 0 {
		t.Fatalf("sqlite baseline migration does not define %s", viewStart)
	}
	rest := sqlText[start:]
	end := strings.Index(rest, "\n\nCREATE INDEX")
	if end < 0 {
		t.Fatal("sqlite baseline migration view definition terminator not found")
	}
	_, err = conn.ExecContext(context.Background(), rest[:end])
	if err != nil {
		t.Fatalf("create branch visible messages view: %v", err)
	}
}
