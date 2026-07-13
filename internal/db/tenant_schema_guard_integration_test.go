//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// nonTenantPublicTables are the public tables that are NOT tenant business
// tables: tooling metadata and the tenants root (whose own id IS the tenant id).
var nonTenantPublicTables = map[string]bool{
	"schema_migrations": true,
	"tenants":           true,
}

// TestTenantSchemaGuard is the G-SCHEMA static guard: it asserts the applied
// schema satisfies the tenant schema contract structurally, so future changes
// cannot silently regress isolation.
func TestTenantSchemaGuard(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	tables := publicBaseTables(t, ctx, pool)
	if len(tables) < 48 {
		t.Fatalf("expected >= 48 public base tables, got %d", len(tables))
	}

	for _, tbl := range tables {
		if nonTenantPublicTables[tbl] {
			continue
		}
		// (1) tenant_id column exists and is NOT NULL.
		var isNullable string
		if err := pool.QueryRow(ctx, `
			SELECT is_nullable FROM information_schema.columns
			 WHERE table_schema='public' AND table_name=$1 AND column_name='tenant_id'`, tbl).Scan(&isNullable); err != nil {
			t.Errorf("tenant table %s: missing tenant_id column: %v", tbl, err)
			continue
		}
		if isNullable != "NO" {
			t.Errorf("tenant table %s: tenant_id must be NOT NULL", tbl)
		}

		// (2) PK leads with tenant_id.
		if col := firstPKColumn(t, ctx, pool, tbl); col != "tenant_id" {
			t.Errorf("tenant table %s: PK must lead with tenant_id, leads with %q", tbl, col)
		}

		// (3) has a root FK (tenant_id) -> tenants(id).
		var hasRootFK bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint con
				  JOIN pg_class rt ON rt.oid = con.confrelid
				  JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY(con.conkey)
				 WHERE con.contype='f' AND con.conrelid = ('public.'||quote_ident($1))::regclass
				   AND rt.relname='tenants' AND a.attname='tenant_id')`, tbl).Scan(&hasRootFK); err != nil {
			t.Fatalf("root FK check %s: %v", tbl, err)
		}
		if !hasRootFK {
			t.Errorf("tenant table %s: missing root FK (tenant_id) -> tenants(id)", tbl)
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
			t.Errorf("tenant table %s: must have RLS enabled+forced (got rls=%v force=%v)", tbl, rls, force)
		}
	}

	// (5) No ON DELETE SET NULL on any composite FK that carries tenant_id.
	var setNullComposite int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid=con.conrelid
		  JOIN pg_namespace n ON n.oid=c.relnamespace
		 WHERE con.contype='f' AND con.confdeltype='n' AND n.nspname='public'
		   AND EXISTS (SELECT 1 FROM pg_attribute a
		               WHERE a.attrelid=con.conrelid AND a.attnum=ANY(con.conkey) AND a.attname='tenant_id')`).Scan(&setNullComposite); err != nil {
		t.Fatalf("set null check: %v", err)
	}
	if setNullComposite != 0 {
		t.Errorf("found %d composite FKs carrying tenant_id with ON DELETE SET NULL (must be RESTRICT/NO ACTION)", setNullComposite)
	}

	// (6) tenants root special case: PK is (id) only, no redundant tenant_id, FORCE RLS.
	var rootHasTenantID bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name='tenants' AND column_name='tenant_id')`).Scan(&rootHasTenantID); err != nil {
		t.Fatalf("root tenant_id check: %v", err)
	}
	if rootHasTenantID {
		t.Error("tenants root must not have a redundant tenant_id column")
	}
	var rootRLS, rootForce bool
	_ = pool.QueryRow(ctx, `SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE oid='public.tenants'::regclass`).Scan(&rootRLS, &rootForce)
	if !rootRLS || !rootForce {
		t.Error("tenants root must have RLS enabled+forced")
	}
}

// TestFenceSecurityGuard is the fence portion of G-RLS-STATIC / G-ROLE: the
// write-fence meta-table must have NO RLS, zero runtime/PUBLIC table ACL, and
// the two helpers must have the exact frozen signatures + SECURITY DEFINER.
func TestFenceSecurityGuard(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	// Fence table: no RLS.
	var fRLS, fForce bool
	if err := pool.QueryRow(ctx, `
		SELECT relrowsecurity, relforcerowsecurity FROM pg_class
		 WHERE oid='app.tenant_write_fences'::regclass`).Scan(&fRLS, &fForce); err != nil {
		t.Fatalf("fence rls: %v", err)
	}
	if fRLS || fForce {
		t.Errorf("app.tenant_write_fences must NOT have RLS (got rls=%v force=%v)", fRLS, fForce)
	}

	// Runtime has zero table privileges on the fence table.
	var runtimeFencePrivs int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.role_table_grants
		 WHERE table_schema='app' AND table_name='tenant_write_fences' AND grantee='memoh_runtime'`).Scan(&runtimeFencePrivs); err != nil {
		t.Fatalf("fence acl: %v", err)
	}
	if runtimeFencePrivs != 0 {
		t.Errorf("memoh_runtime must have zero table privileges on app.tenant_write_fences, got %d", runtimeFencePrivs)
	}

	// Helper signatures: assert (void, SECURITY DEFINER), matches (boolean,
	// SECURITY DEFINER); current_tenant_id (uuid), current_fencing_token (bigint).
	checkFn := func(name, wantRet string, wantSecdef bool) {
		var retType string
		var secdef bool
		if err := pool.QueryRow(ctx, `
			SELECT pg_catalog.format_type(p.prorettype, NULL), p.prosecdef
			  FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
			 WHERE n.nspname='app' AND p.proname=$1`, name).Scan(&retType, &secdef); err != nil {
			t.Errorf("helper %s: %v", name, err)
			return
		}
		if retType != wantRet {
			t.Errorf("app.%s must return %s, returns %s", name, wantRet, retType)
		}
		if secdef != wantSecdef {
			t.Errorf("app.%s SECURITY DEFINER = %v, want %v", name, secdef, wantSecdef)
		}
	}
	checkFn("assert_tenant_write_fence", "void", true)
	checkFn("tenant_write_fence_matches", "boolean", true)
	checkFn("current_tenant_id", "uuid", false)
	checkFn("current_fencing_token", "bigint", false)

	// Runtime must NOT be BYPASSRLS and must not be a superuser.
	var bypass, super bool
	if err := pool.QueryRow(ctx, `
		SELECT rolbypassrls, rolsuper FROM pg_roles WHERE rolname='memoh_runtime'`).Scan(&bypass, &super); err != nil {
		t.Fatalf("runtime role attrs: %v", err)
	}
	if bypass {
		t.Error("memoh_runtime must NOT have BYPASSRLS")
	}
	if super {
		t.Error("memoh_runtime must NOT be a superuser")
	}
}

// TestRLSPolicyGuard is the policy portion of G-RLS-STATIC: every tenant table
// has the four per-command policies, and the write policies call the lock-free
// fence-matches helper (not the lock-holding assert).
func TestRLSPolicyGuard(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	for _, tbl := range publicBaseTables(t, ctx, pool) {
		if nonTenantPublicTables[tbl] {
			continue
		}
		var policyCount int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM pg_policies WHERE schemaname='public' AND tablename=$1`, tbl).Scan(&policyCount); err != nil {
			t.Fatalf("policy count %s: %v", tbl, err)
		}
		if policyCount < 4 {
			t.Errorf("tenant table %s: expected >= 4 per-command policies, got %d", tbl, policyCount)
		}

		// Write policies (INSERT/UPDATE/DELETE) must reference the lock-free
		// tenant_write_fence_matches() and must NOT reference the lock-holding
		// assert_tenant_write_fence().
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
			if strings.Contains(body, "assert_tenant_write_fence") {
				t.Errorf("%s policy %s must not call the lock-holding assert_tenant_write_fence()", tbl, cmd)
			}
			if cmd == "INSERT" || cmd == "UPDATE" || cmd == "DELETE" {
				if !strings.Contains(body, "tenant_write_fence_matches") {
					t.Errorf("%s %s policy must call tenant_write_fence_matches()", tbl, cmd)
				}
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

func firstPKColumn(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) string {
	t.Helper()
	var col string
	if err := pool.QueryRow(ctx, `
		SELECT a.attname FROM pg_constraint con
		  JOIN pg_attribute a ON a.attrelid=con.conrelid AND a.attnum=con.conkey[1]
		 WHERE con.contype='p' AND con.conrelid=('public.'||quote_ident($1))::regclass`, table).Scan(&col); err != nil {
		return ""
	}
	return col
}

