//go:build integration

package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rlsConn returns a connection running as a non-superuser test role. Roles are
// test setup only; production migrations intentionally do not create or alter
// cluster-wide roles.
func rlsConn(t *testing.T, ownerPool *pgxpool.Pool, dsn string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	role := fmt.Sprintf("memoh_rls_test_%d_%d", os.Getpid(), teamTestDBSeq.Add(1))
	if _, err := ownerPool.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS"); err != nil {
		t.Fatalf("create RLS test role: %v", err)
	}
	grants := []string{
		"GRANT USAGE ON SCHEMA public, app TO " + role,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + role,
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + role,
		"GRANT EXECUTE ON FUNCTION app.current_team_id() TO " + role,
	}
	for _, grant := range grants {
		if _, err := ownerPool.Exec(ctx, grant); err != nil {
			t.Fatalf("grant RLS test role: %v", err)
		}
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for RLS test: %v", err)
	}
	if _, err := conn.Exec(ctx, "SET ROLE "+role); err != nil {
		_ = conn.Close(ctx)
		t.Fatalf("set RLS test role: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(ctx)
		_, _ = ownerPool.Exec(ctx, "DROP OWNED BY "+role)
		_, _ = ownerPool.Exec(ctx, "DROP ROLE IF EXISTS "+role)
	})
	return conn
}

func TestTeamRLSForceEnabled(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	rows, err := pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relkind IN ('r', 'p') AND n.nspname = 'public'
		   AND (c.relname='teams' OR EXISTS (
		       SELECT 1 FROM pg_constraint con
		       JOIN pg_class parent ON parent.oid=con.confrelid
		        WHERE con.conrelid=c.oid AND con.contype='f' AND parent.relname='teams'))`)
	if err != nil {
		t.Fatalf("enumerate team tables: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var enabled, forced bool
		if err := rows.Scan(&name, &enabled, &forced); err != nil {
			t.Fatalf("scan team table: %v", err)
		}
		if !enabled || !forced {
			t.Errorf("table %s must have RLS enabled and forced", name)
		}
	}
}

func TestTeamRLSDynamicIsolation(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := teamMigrationDSN(t)
	const t1 = "00000000-0000-0000-0000-000000000001"
	const t2 = "00000000-0000-0000-0000-0000000000f2"

	if _, err := pool.Exec(ctx, `INSERT INTO teams (id, slug) VALUES ($1, 't2')`, t2); err != nil {
		t.Fatalf("seed team2: %v", err)
	}
	rc := rlsConn(t, pool, dsn)

	writeAs := func(teamID string, fn func(pgx.Tx) error) error {
		tx, err := rc.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, "SELECT set_config('app.team_id', $1, true)", teamID); err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	for _, tc := range []struct{ teamID, name string }{{t1, "p1"}, {t2, "p1"}} {
		if err := writeAs(tc.teamID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO providers (team_id, name, client_type) VALUES ($1, $2, 'openai-completions')`,
				tc.teamID, tc.name)
			return err
		}); err != nil {
			t.Fatalf("insert provider for %s: %v", tc.teamID, err)
		}
	}

	readCount := func(teamID string) int {
		tx, err := rc.Begin(ctx)
		if err != nil {
			t.Fatalf("begin read: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, "SELECT set_config('app.team_id', $1, true)", teamID); err != nil {
			t.Fatalf("bind team: %v", err)
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM providers").Scan(&count); err != nil {
			t.Fatalf("count providers: %v", err)
		}
		return count
	}
	if got := readCount(t1); got != 1 {
		t.Errorf("team 1 saw %d providers, want 1", got)
	}
	if got := readCount(t2); got != 1 {
		t.Errorf("team 2 saw %d providers, want 1", got)
	}

	err := writeAs(t1, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO providers (team_id, name, client_type) VALUES ($1, 'cross', 'openai-completions')`, t2)
		return err
	})
	if sqlState(err) != "42501" {
		t.Errorf("cross-team insert SQLSTATE = %q, want 42501", sqlState(err))
	}

	tx, err := rc.Begin(ctx)
	if err != nil {
		t.Fatalf("begin missing-context write: %v", err)
	}
	_, writeErr := tx.Exec(ctx,
		`INSERT INTO providers (team_id, name, client_type) VALUES ($1, 'missing', 'openai-completions')`, t1)
	_ = tx.Rollback(ctx)
	if sqlState(writeErr) != "42501" {
		t.Errorf("missing team context SQLSTATE = %q, want 42501", sqlState(writeErr))
	}
}
