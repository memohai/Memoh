//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// nonTeamPublicTables are the public tables that are NOT team business
// tables: tooling metadata and the teams root (whose own id IS the team id).
var nonTeamPublicTables = map[string]bool{
	"schema_migrations": true,
	"teams":             true,
}

// TestTeamViewGuard verifies that views over team tables do not bypass RLS.
// Every public view must run with
// security_invoker = true (so the caller's RLS applies), project a team_id
// column (so consumers can scope explicitly and the isolation is auditable).
func TestTeamViewGuard(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	rows, err := pool.Query(ctx, `
		SELECT c.relname,
		       COALESCE((SELECT option_value FROM pg_options_to_table(c.reloptions)
		                  WHERE option_name='security_invoker'), 'false') AS security_invoker
		  FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		 WHERE c.relkind='v' AND n.nspname='public'`)
	if err != nil {
		t.Fatalf("list views: %v", err)
	}
	type view struct{ name, secInvoker string }
	var views []view
	for rows.Next() {
		var v view
		_ = rows.Scan(&v.name, &v.secInvoker)
		views = append(views, v)
	}
	rows.Close()

	for _, v := range views {
		// (1) security_invoker must be true.
		if v.secInvoker != "true" {
			t.Errorf("view %s must be WITH (security_invoker = true) to respect caller RLS, got %q", v.name, v.secInvoker)
		}
		// (2) must project a team_id column.
		var hasTeamID bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM information_schema.columns
				WHERE table_schema='public' AND table_name=$1 AND column_name='team_id')`, v.name).Scan(&hasTeamID); err != nil {
			t.Fatalf("view %s columns: %v", v.name, err)
		}
		if !hasTeamID {
			t.Errorf("view %s must project team_id", v.name)
		}
	}
}

// TestTeamSchemaGuard asserts the team schema invariants structurally.
func TestTeamSchemaGuard(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	tables := publicBaseTables(t, ctx, pool)
	if len(tables) < 48 {
		t.Fatalf("expected >= 48 public base tables, got %d", len(tables))
	}

	for _, tbl := range tables {
		if nonTeamPublicTables[tbl] {
			continue
		}
		// (1) team_id column exists and is NOT NULL.
		var isNullable string
		if err := pool.QueryRow(ctx, `
			SELECT is_nullable FROM information_schema.columns
			 WHERE table_schema='public' AND table_name=$1 AND column_name='team_id'`, tbl).Scan(&isNullable); err != nil {
			t.Errorf("team table %s: missing team_id column: %v", tbl, err)
			continue
		}
		if isNullable != "NO" {
			t.Errorf("team table %s: team_id must be NOT NULL", tbl)
		}

		// (2) Existing PK remains stable and has a team-prefixed helper key.
		var helperKeys int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM pg_constraint
			 WHERE conrelid=('public.'||quote_ident($1))::regclass
			   AND contype='u' AND conname LIKE 'memoh_team_key_%'`, tbl).Scan(&helperKeys); err != nil {
			t.Fatalf("team key check %s: %v", tbl, err)
		}
		if helperKeys != 1 {
			t.Errorf("team table %s: expected one team-prefixed helper key, got %d", tbl, helperKeys)
		}

		// (3) has a root FK (team_id) -> teams(id).
		var hasRootFK bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint con
				  JOIN pg_class rt ON rt.oid = con.confrelid
				  JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY(con.conkey)
				 WHERE con.contype='f' AND con.conrelid = ('public.'||quote_ident($1))::regclass
				   AND rt.relname='teams' AND a.attname='team_id')`, tbl).Scan(&hasRootFK); err != nil {
			t.Fatalf("root FK check %s: %v", tbl, err)
		}
		if !hasRootFK {
			t.Errorf("team table %s: missing root FK (team_id) -> teams(id)", tbl)
		}

		// (4) RLS enabled + forced.
		var rls, force bool
		if err := pool.QueryRow(ctx, `
			SELECT c.relrowsecurity, c.relforcerowsecurity FROM pg_class c
			  JOIN pg_namespace n ON n.oid=c.relnamespace
			 WHERE n.nspname='public' AND c.relname=$1`, tbl).Scan(&rls, &force); err != nil {
			t.Fatalf("rls check %s: %v", tbl, err)
		}
		if !rls || !force {
			t.Errorf("team table %s: must have RLS enabled+forced (got rls=%v force=%v)", tbl, rls, force)
		}

	}

	// (5) SET NULL may clear the original reference, but never team_id.
	var unsafeSetNull int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid=con.conrelid
		  JOIN pg_namespace n ON n.oid=c.relnamespace
		 WHERE con.contype='f' AND con.confdeltype='n' AND n.nspname='public'
		   AND (con.confdelsetcols IS NULL OR EXISTS (
		       SELECT 1 FROM pg_attribute a
		        WHERE a.attrelid=con.conrelid AND a.attnum=ANY(con.confdelsetcols)
		          AND a.attname='team_id'))`).Scan(&unsafeSetNull); err != nil {
		t.Fatalf("set null check: %v", err)
	}
	if unsafeSetNull != 0 {
		t.Errorf("found %d SET NULL FKs that can clear team_id", unsafeSetNull)
	}

	// (6) teams root special case: PK is (id) only, no redundant team_id, FORCE RLS.
	var rootHasTeamID bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='teams' AND column_name='team_id')`).Scan(&rootHasTeamID); err != nil {
		t.Fatalf("root team_id check: %v", err)
	}
	if rootHasTeamID {
		t.Error("teams root must not have a redundant team_id column")
	}
	var rootRLS, rootForce bool
	_ = pool.QueryRow(ctx, `SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE oid='public.teams'::regclass`).Scan(&rootRLS, &rootForce)
	if !rootRLS || !rootForce {
		t.Error("teams root must have RLS enabled+forced")
	}

	// (7) NULLS NOT DISTINCT must be preserved on bot_acl_rules_unique_target
	// (the composite-key rebuild must not silently widen it to a plain UNIQUE,
	// which would let duplicate ACL rules with NULL key columns through).
	var nnd bool
	if err := pool.QueryRow(ctx, `
		SELECT i.indnullsnotdistinct FROM pg_constraint con
		  JOIN pg_index i ON i.indexrelid = con.conindid
		 WHERE con.conname = 'bot_acl_rules_unique_target'`).Scan(&nnd); err != nil {
		t.Fatalf("nulls-not-distinct check: %v", err)
	}
	if !nnd {
		t.Error("bot_acl_rules_unique_target must be UNIQUE NULLS NOT DISTINCT (semantics widened)")
	}
}

// TestRLSPolicyGuard verifies every team table has four team-only policies.
func TestRLSPolicyGuard(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	for _, tbl := range publicBaseTables(t, ctx, pool) {
		if nonTeamPublicTables[tbl] {
			continue
		}
		var policyCount int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM pg_policies WHERE schemaname='public' AND tablename=$1`, tbl).Scan(&policyCount); err != nil {
			t.Fatalf("policy count %s: %v", tbl, err)
		}
		if policyCount < 4 {
			t.Errorf("team table %s: expected >= 4 per-command policies, got %d", tbl, policyCount)
		}

		// Every policy must resolve the team from the database context and must
		// not depend on deployment-specific write-fencing helpers.
		rows, err := pool.Query(ctx, `
			SELECT cmd, COALESCE(qual,''), COALESCE(with_check,'')
			  FROM pg_policies WHERE schemaname='public' AND tablename=$1`, tbl)
		if err != nil {
			t.Fatalf("policies %s: %v", tbl, err)
		}
		for rows.Next() {
			var cmd, qual, withCheck string
			_ = rows.Scan(&cmd, &qual, &withCheck)
			body := qual + " " + withCheck
			if !strings.Contains(body, "current_team_id") {
				t.Errorf("%s policy %s must call current_team_id()", tbl, cmd)
			}
			if strings.Contains(body, "fence") {
				t.Errorf("%s policy %s must not contain deployment-specific fencing", tbl, cmd)
			}
		}
		rows.Close()
	}
}

// --- helpers ---

func publicBaseTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		 WHERE c.relkind='r' AND n.nspname='public' ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var out []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		out = append(out, n)
	}
	rows.Close()
	return out
}
