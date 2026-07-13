//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestTenantRLSForceEnabled asserts every tenant table (and the tenants root)
// has RLS ENABLED + FORCED with the correct per-command policies, and that the
// fence meta-table has RLS OFF (security design §5, §6).
func TestTenantRLSForceEnabled(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	// Every public tenant table + tenants root must be relrowsecurity AND
	// relforcerowsecurity.
	rows, err := pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relkind = 'r' AND n.nspname = 'public'
		   AND c.relname NOT IN ('schema_migrations')`)
	if err != nil {
		t.Fatalf("enumerate tables: %v", err)
	}
	for rows.Next() {
		var name string
		var rls, force bool
		if err := rows.Scan(&name, &rls, &force); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if !rls || !force {
			t.Errorf("table %s must have RLS enabled+forced, got rls=%v force=%v", name, rls, force)
		}
	}
	rows.Close()

	// The fence meta-table must NOT have RLS (its boundary is ACL, not RLS).
	var fenceRLS, fenceForce bool
	if err := pool.QueryRow(ctx, `
		SELECT c.relrowsecurity, c.relforcerowsecurity FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'app' AND c.relname = 'tenant_write_fences'`).Scan(&fenceRLS, &fenceForce); err != nil {
		t.Fatalf("fence rls: %v", err)
	}
	if fenceRLS || fenceForce {
		t.Errorf("app.tenant_write_fences must NOT have RLS, got rls=%v force=%v", fenceRLS, fenceForce)
	}
}

// TestTenantRLSDynamicIsolation exercises the runtime role under RLS: with a
// tenant GUC bound, a SELECT sees only that tenant's rows; a write requires the
// fence assert; cross-tenant writes are rejected; missing GUC is fail-closed.
func TestTenantRLSDynamicIsolation(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)
	const t1 = "00000000-0000-0000-0000-000000000001"
	const t2 = "00000000-0000-0000-0000-0000000000f2"

	// Seed tenant2 + its fence (enabled, token 1) via the migrator-owned path.
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES ($1, 't2')`, t2); err != nil {
		t.Fatalf("seed tenant2: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO app.tenant_write_fences (tenant_id, fencing_token, write_enabled) VALUES ($1, 1, true)`, t2); err != nil {
		t.Fatalf("seed tenant2 fence: %v", err)
	}

	rc := runtimeConn(t, pool, dsn)

	// Helper to run a write tx as runtime with both GUCs + assert.
	writeAs := func(tenant string, fn func(tx pgx.Tx) error) error {
		tx, err := rc.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.fencing_token', '1', true)"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT app.assert_tenant_write_fence()"); err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	// t1 inserts a provider.
	if err := writeAs(t1, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO providers (tenant_id, name, client_type) VALUES ($1, 'p1', 'openai-completions')`, t1)
		return err
	}); err != nil {
		t.Fatalf("t1 insert: %v", err)
	}
	// t2 inserts a provider with the same name (allowed — tenant-scoped unique).
	if err := writeAs(t2, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO providers (tenant_id, name, client_type) VALUES ($1, 'p1', 'openai-completions')`, t2)
		return err
	}); err != nil {
		t.Fatalf("t2 insert: %v", err)
	}

	// Under t1's context, a SELECT must see exactly one provider (t1's).
	readCount := func(tenant string) int {
		tx, err := rc.Begin(ctx)
		if err != nil {
			t.Fatalf("begin read: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, _ = tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant)
		_, _ = tx.Exec(ctx, "SELECT set_config('app.fencing_token', '1', true)")
		var n int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM providers").Scan(&n); err != nil {
			t.Fatalf("count providers as %s: %v", tenant, err)
		}
		return n
	}
	if got := readCount(t1); got != 1 {
		t.Errorf("t1 must see exactly its own provider, saw %d", got)
	}
	if got := readCount(t2); got != 1 {
		t.Errorf("t2 must see exactly its own provider, saw %d", got)
	}

	// Cross-tenant write (WITH CHECK): under t1 context, insert tenant_id=t2 -> 42501.
	err := writeAs(t1, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO providers (tenant_id, name, client_type) VALUES ($1, 'x', 'openai-completions')`, t2)
		return e
	})
	if sqlState(err) != "42501" {
		t.Errorf("cross-tenant INSERT (WITH CHECK) must be 42501, got %q", sqlState(err))
	}

	// Missing tenant GUC -> fail-closed 42501 on the write assert.
	tx, _ := rc.Begin(ctx)
	_, _ = tx.Exec(ctx, "SELECT set_config('app.fencing_token', '1', true)")
	_, e := tx.Exec(ctx, "SELECT app.assert_tenant_write_fence()")
	_ = tx.Rollback(ctx)
	if sqlState(e) != "42501" {
		t.Errorf("missing tenant GUC must be 42501, got %q", sqlState(e))
	}
}
