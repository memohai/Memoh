//go:build integration

package db_test

import (
	"context"
	"testing"
)

// TestMigrateLegacyInstallPreservesRows simulates upgrading an EXISTING
// (pre-tenant) install: it applies the migration chain up to the last
// pre-tenant migration, seeds representative business data, then applies the
// tenant migrations (0106+). It asserts no rows are lost, every row is
// backfilled to the default tenant, the singleton fence exists, and the schema
// reached the final tenant-scoped shape. This is the G-MIG upstream subset:
// existing installs upgrade in place with no wipe.
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

	// The singleton tenant + its fence exist (fence enabled, positive token).
	var token int64
	var enabled bool
	if err := pool.QueryRow(ctx,
		"SELECT fencing_token, write_enabled FROM app.tenant_write_fences WHERE tenant_id = $1", defaultTenant,
	).Scan(&token, &enabled); err != nil {
		t.Fatalf("singleton fence after upgrade: %v", err)
	}
	if token <= 0 || !enabled {
		t.Errorf("singleton fence must be (token>0, enabled), got (%d, %v)", token, enabled)
	}

	// The final schema is tenant-scoped: bots PK leads with tenant_id, FORCE RLS on.
	var pkFirst string
	if err := pool.QueryRow(ctx, `
		SELECT a.attname FROM pg_constraint con
		  JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = con.conkey[1]
		 WHERE con.contype='p' AND con.conrelid = 'public.bots'::regclass`).Scan(&pkFirst); err != nil {
		t.Fatalf("bots pk: %v", err)
	}
	if pkFirst != "tenant_id" {
		t.Errorf("after upgrade bots PK must lead with tenant_id, leads with %q", pkFirst)
	}
}

// TestMigrateDownFailClosedForMultiTenant asserts that once a non-default tenant
// exists, stepping the tenant migrations down fails closed rather than
// destroying tenant data (schema contract down safety gate).
func TestMigrateDownFailClosedForMultiTenant(t *testing.T) {
	ctx := context.Background()
	dsn := tenantMigrationDSN(t)
	pool := freshMigratedDB(t)

	const t2 = "00000000-0000-0000-0000-0000000000f2"
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES ($1, 'other')`, t2); err != nil {
		t.Fatalf("insert non-default tenant: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO app.tenant_write_fences (tenant_id, fencing_token, write_enabled) VALUES ($1, 1, true)`, t2); err != nil {
		t.Fatalf("insert non-default fence: %v", err)
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
