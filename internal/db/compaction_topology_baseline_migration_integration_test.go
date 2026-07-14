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

func TestCompactionTopologyBaselineMigrationDownRemovesTopologyObjectsPostgresPath(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect baseline migration postgres: %v", err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin baseline migration test: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "compaction_topology_baseline_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create baseline schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema+", public"); err != nil {
		t.Fatalf("set baseline search path: %v", err)
	}
	if _, err := tx.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")); err != nil {
		t.Fatalf("apply baseline up: %v", err)
	}
	if _, err := tx.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0001_init.down.sql")); err != nil {
		t.Fatalf("apply baseline down: %v", err)
	}

	for _, relation := range []string{
		"bot_history_message_compact_validations",
		"bot_history_topology_counters",
		"bot_history_topology_positions",
		"bot_history_topology_pending",
		"bot_history_message_compact_topology",
	} {
		var found *string
		if err := tx.QueryRow(ctx, `SELECT to_regclass($1)::text`, schema+"."+relation).Scan(&found); err != nil {
			t.Fatalf("check baseline relation %s: %v", relation, err)
		}
		if found != nil {
			t.Fatalf("baseline down retained relation %s", relation)
		}
	}
	for _, function := range []string{
		"enqueue_history_topology_position(uuid,bigint)",
		"record_history_message_topology_change()",
		"flush_history_topology_positions()",
		"cleanup_history_topology_session()",
	} {
		var count int
		if err := tx.QueryRow(ctx, `
SELECT COUNT(*)
FROM pg_proc function
JOIN pg_namespace namespace ON namespace.oid = function.pronamespace
WHERE namespace.nspname = $1
  AND function.oid = to_regprocedure($2)
`, schema, schema+"."+function).Scan(&count); err != nil {
			t.Fatalf("check baseline function %s: %v", function, err)
		}
		if count != 0 {
			t.Fatalf("baseline down retained function %s", function)
		}
	}
}
