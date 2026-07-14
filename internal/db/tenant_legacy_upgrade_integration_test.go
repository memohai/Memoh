//go:build integration

package db_test

import (
	"context"
	"testing"
)

// TestMigrateLegacyInstallPreservesRows simulates upgrading an EXISTING
// (pre-tenant) install: it applies the migration chain up to the last
// pre-tenant migration, seeds representative business data, then applies the
// the consolidated tenant migration. It asserts no rows are lost, every row is
// backfilled to the default tenant, and the schema reached the final
// tenant-scoped shape. Existing installs upgrade in place without a wipe.
func TestMigrateLegacyInstallPreservesRows(t *testing.T) {
	ctx := context.Background()
	dsn := tenantMigrationDSN(t)
	pool := resetToEmpty(t)

	tenantSteps := countTenantMigrations(t)

	// Apply the chain up to (but not including) the tenant migrations — the
	// "legacy install" state.
	stepUpToPreTenant(t, dsn, tenantSteps)

	// tenants must not exist yet (pre-tenant baseline).
	var tenantsExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='tenants')`).Scan(&tenantsExists); err != nil {
		t.Fatalf("check tenants pre-upgrade: %v", err)
	}
	if tenantsExists {
		t.Fatal("tenants must not exist before the tenant migrations")
	}

	// Seed representative legacy business data: a user + a bot owned by it.
	var userID, botID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, is_active, metadata) VALUES ('legacy-user', 'u@example.com', true, '{}') RETURNING id`,
	).Scan(&userID); err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO bots (owner_user_id, name, status, metadata) VALUES ($1, 'legacy-bot', 'ready', '{}') RETURNING id`, userID,
	).Scan(&botID); err != nil {
		t.Fatalf("seed legacy bot: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO bot_sessions (bot_id, channel_type, title, metadata) VALUES ($1, 'local', 'legacy session', '{}')`, botID,
	); err != nil {
		t.Fatalf("seed legacy session: %v", err)
	}

	// Apply the tenant migrations (the upgrade).
	stepUp(t, dsn, tenantSteps)

	// Rows preserved.
	assertCount := func(table string, want int) {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != want {
			t.Errorf("%s row count = %d, want %d (upgrade must preserve rows)", table, n, want)
		}
	}
	assertCount("users", 1)
	assertCount("bots", 1)
	assertCount("bot_sessions", 1)

	// Every seeded row is backfilled to the default tenant.
	const defaultTenant = "00000000-0000-0000-0000-000000000001"
	for _, table := range []string{"users", "bots", "bot_sessions"} {
		var nonDefault int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM "+table+" WHERE tenant_id IS DISTINCT FROM $1", defaultTenant,
		).Scan(&nonDefault); err != nil {
			t.Fatalf("check backfill %s: %v", table, err)
		}
		if nonDefault != 0 {
			t.Errorf("%s has %d rows not backfilled to the default tenant", table, nonDefault)
		}
	}

	// The final schema keeps the existing PK and adds a tenant-prefixed unique
	// key that composite foreign keys can reference.
	var tenantKeyCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		 WHERE con.contype='u' AND con.conrelid='public.bots'::regclass
		   AND con.conname LIKE 'memoh_tenant_key_%'
		   AND (SELECT a.attname FROM pg_attribute a
		         WHERE a.attrelid=con.conrelid AND a.attnum=con.conkey[1])='tenant_id'`).Scan(&tenantKeyCount); err != nil {
		t.Fatalf("bots tenant key: %v", err)
	}
	if tenantKeyCount != 1 {
		t.Errorf("after upgrade bots must have one tenant-prefixed key, got %d", tenantKeyCount)
	}
}

// TestMigrateDownFailClosedForMultiTenant asserts that once a non-default tenant
// exists, stepping the tenant migrations down fails closed rather than
// destroying tenant data.
func TestMigrateDownFailClosedForMultiTenant(t *testing.T) {
	ctx := context.Background()
	dsn := tenantMigrationDSN(t)
	pool := freshMigratedDB(t)

	const t2 = "00000000-0000-0000-0000-0000000000f2"
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES ($1, 'other')`, t2); err != nil {
		t.Fatalf("insert non-default tenant: %v", err)
	}
	// Stepping the tenant migrations down must fail closed.
	if downErr := tryStepDown(t, dsn, countTenantMigrations(t)); downErr == nil {
		t.Fatal("stepping tenant migrations down must fail closed with a non-default tenant present")
	}
}

// TestMigrateDownSingletonSafe asserts a clean singleton database can step the
// tenant migrations down and back up (reversibility on the supported down path).
func TestMigrateDownSingletonSafe(t *testing.T) {
	ctx := context.Background()
	dsn := tenantMigrationDSN(t)
	pool := freshMigratedDB(t)

	steps := countTenantMigrations(t)
	stepDown(t, dsn, steps)
	var tenantsExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='tenants')`).Scan(&tenantsExists); err != nil {
		t.Fatalf("check tenants after down: %v", err)
	}
	if tenantsExists {
		t.Error("clean singleton must be able to drop the tenant schema on down")
	}
	stepUp(t, dsn, steps)
}

func TestTenantMigrationsLeaveUserTablesUntouched(t *testing.T) {
	ctx := context.Background()
	dsn := tenantMigrationDSN(t)
	pool := resetToEmpty(t)
	tenantSteps := countTenantMigrations(t)
	stepUpToPreTenant(t, dsn, tenantSteps)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE public.user_extension_data (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id uuid,
			payload text NOT NULL
		);
		INSERT INTO public.user_extension_data (tenant_id, payload)
		VALUES ('00000000-0000-0000-0000-0000000000ee', 'preserve me')`); err != nil {
		t.Fatalf("create user table: %v", err)
	}

	stepUp(t, dsn, tenantSteps)

	var rls, forced bool
	if err := pool.QueryRow(ctx, `
		SELECT relrowsecurity, relforcerowsecurity
		  FROM pg_class WHERE oid='public.user_extension_data'::regclass`).Scan(&rls, &forced); err != nil {
		t.Fatalf("read user table RLS: %v", err)
	}
	if rls || forced {
		t.Errorf("tenant migrations changed user table RLS: enabled=%v forced=%v", rls, forced)
	}
	var tenantFKs int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		 JOIN pg_class parent ON parent.oid=con.confrelid
		 WHERE con.conrelid='public.user_extension_data'::regclass
		   AND con.contype='f' AND parent.relname='tenants'`).Scan(&tenantFKs); err != nil {
		t.Fatalf("read user table FKs: %v", err)
	}
	if tenantFKs != 0 {
		t.Errorf("tenant migrations added %d tenant FKs to user table", tenantFKs)
	}
	var payload string
	if err := pool.QueryRow(ctx, `SELECT payload FROM public.user_extension_data`).Scan(&payload); err != nil {
		t.Fatalf("read user table row: %v", err)
	}
	if payload != "preserve me" {
		t.Errorf("user table payload = %q", payload)
	}
}
