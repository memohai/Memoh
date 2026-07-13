//go:build integration

package db_test

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	embeddeddb "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/tenant"
)

// tenantMigrationDSN returns the integration DSN or fails the test. Per the
// tenant-isolation verification plan, missing infra must FAIL the required
// check (not silently Skip) — so these tests use t.Fatal, and are gated behind
// the `integration` build tag which CI must run with a real database.
func tenantMigrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("TEST_POSTGRES_DSN must be set for tenant migration integration tests")
	}
	return dsn
}

// pgConfigFromDSN parses a libpq DSN/URL into the repo's PostgresConfig.
func pgConfigFromDSN(t *testing.T, dsn string) config.PostgresConfig {
	t.Helper()
	pc, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	ssl := "disable"
	if pc.TLSConfig != nil {
		ssl = "require"
	}
	return config.PostgresConfig{
		Host:     pc.Host,
		Port:     int(pc.Port),
		User:     pc.User,
		Password: pc.Password,
		Database: pc.Database,
		SSLMode:  ssl,
	}
}

// postgresMigrationsFS returns the embedded PostgreSQL migrations sub-tree.
func postgresMigrationsFS(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(embeddeddb.MigrationsFS, "postgres/migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	return sub
}

// freshMigratedDB drops and recreates the public schema on the target database,
// then applies the full PostgreSQL migration chain (0001..head). It returns a
// connected pool. Callers get an empty database migrated to head.
func freshMigratedDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := tenantMigrationDSN(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// Reset to a pristine state so migrate up starts from empty. Drop the
	// public + app schemas and the tenant roles created by 0107, since those
	// live outside public and would otherwise survive across test runs.
	reset := `
		DROP SCHEMA IF EXISTS public CASCADE;
		DROP SCHEMA IF EXISTS app CASCADE;
		CREATE SCHEMA public;
	`
	if _, err := pool.Exec(ctx, reset); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	for _, role := range []string{"memoh_runtime", "memoh_migrator", "memoh_break_glass", "memoh_owner"} {
		// Reassign/drop owned objects first is unnecessary after schema drops;
		// ignore "does not exist" and dependency errors defensively.
		_, _ = pool.Exec(ctx, "DROP ROLE IF EXISTS "+role)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := db.RunMigrate(logger, pgConfigFromDSN(t, dsn), postgresMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return pool
}

func TestSingletonTenantSeededAfterMigrate(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM tenants").Scan(&count); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 seeded tenant, got %d", count)
	}

	var id, slug string
	if err := pool.QueryRow(ctx, "SELECT id::text, slug FROM tenants").Scan(&id, &slug); err != nil {
		t.Fatalf("select tenant: %v", err)
	}
	if id != tenant.DefaultTenantID {
		t.Fatalf("seeded tenant id = %q, want DefaultTenantID %q", id, tenant.DefaultTenantID)
	}
	if slug != "default" {
		t.Fatalf("seeded tenant slug = %q, want %q", slug, "default")
	}

	// tenants root must NOT carry a redundant tenant_id column (it is the root:
	// its own id IS the tenant id).
	var hasTenantID bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'tenants' AND column_name = 'tenant_id'
		)`).Scan(&hasTenantID); err != nil {
		t.Fatalf("check tenants columns: %v", err)
	}
	if hasTenantID {
		t.Fatal("tenants root must not have a redundant tenant_id column")
	}
}

// stepDown rolls back exactly n migration steps using the golang-migrate library
// directly. The repo's RunMigrate("down") rolls back ALL migrations, so tests
// that need single-step reversibility use this helper instead.
func stepDown(t *testing.T, dsn string, n int) {
	t.Helper()
	src, err := iofs.New(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("iofs: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Steps(-n); err != nil {
		t.Fatalf("step down %d: %v", n, err)
	}
}

// stepUp applies exactly n migration steps using the golang-migrate library.
func stepUp(t *testing.T, dsn string, n int) {
	t.Helper()
	src, err := iofs.New(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("iofs: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Steps(n); err != nil {
		t.Fatalf("step up %d: %v", n, err)
	}
}

// tryStepDown steps n migrations down and RETURNS any error (instead of failing
// the test), so callers can assert a fail-closed down gate.
func tryStepDown(t *testing.T, dsn string, n int) error {
	t.Helper()
	src, err := iofs.New(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("iofs: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() { _, _ = m.Close() }()
	return m.Steps(-n)
}

// resetToEmpty drops and recreates a pristine public schema (and the app schema
// + tenant roles) WITHOUT applying any migrations, returning a connected pool.
func resetToEmpty(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := tenantMigrationDSN(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		DROP SCHEMA IF EXISTS public CASCADE;
		DROP SCHEMA IF EXISTS app CASCADE;
		CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	for _, role := range []string{"memoh_runtime", "memoh_migrator", "memoh_break_glass", "memoh_owner"} {
		_, _ = pool.Exec(ctx, "DROP ROLE IF EXISTS "+role)
	}
	return pool
}

// stepUpToPreTenant applies the migration chain up to (but not including) the
// tenantSteps tenant migrations — i.e. the "legacy install" pre-tenant state.
func stepUpToPreTenant(t *testing.T, dsn string, tenantSteps int) {
	t.Helper()
	src, err := iofs.New(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("iofs: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() { _, _ = m.Close() }()
	total := countAllMigrations(t)
	if err := m.Steps(total - tenantSteps); err != nil {
		t.Fatalf("step up to pre-tenant (%d steps): %v", total-tenantSteps, err)
	}
}

// countAllMigrations returns the number of up migrations in the embedded FS.
func countAllMigrations(t *testing.T) int {
	t.Helper()
	entries, err := fs.ReadDir(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			n++
		}
	}
	return n
}

// TestTenantChainReversible verifies the full tenant migration chain
// (0106..0109) is cleanly reversible: a full step-down of the tenant migrations
// removes all tenant objects, and a step-up re-applies them. It also verifies
// the 0106 down safety gate refuses to drop the tenants root when a non-default
// tenant exists.
func TestTenantChainReversible(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)

	// Number of tenant migrations layered on top of the base chain (>= 0106),
	// computed from the embedded migration files so adding a migration doesn't
	// break this test.
	tenantSteps := countTenantMigrations(t)

	// Step the tenant migrations down; tenants + app schema must be gone.
	stepDown(t, dsn, tenantSteps)
	var tenantsExists, appExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='tenants')`).Scan(&tenantsExists); err != nil {
		t.Fatalf("check tenants after step down: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name='app')`).Scan(&appExists); err != nil {
		t.Fatalf("check app schema after step down: %v", err)
	}
	if tenantsExists {
		t.Error("tenants root must be dropped after stepping down the tenant migrations")
	}
	if appExists {
		t.Error("app schema must be dropped after stepping down the tenant migrations")
	}

	// Re-apply and confirm the composite-key final state is restored.
	stepUp(t, dsn, tenantSteps)
	var setNull int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE con.contype='f' AND con.confdeltype='n' AND n.nspname='public'`).Scan(&setNull); err != nil {
		t.Fatalf("count set null after re-up: %v", err)
	}
	if setNull != 0 {
		t.Errorf("after re-up expected 0 SET NULL FKs, got %d", setNull)
	}
}

// TestTenantsRootDownSafetyGate verifies 0106's down safety gate: when a
// non-default tenant exists, stepping the tenant migrations down must fail
// closed rather than dropping tenant data.
func TestTenantsRootDownSafetyGate(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)

	// Seed a second tenant + its fence so the DB is genuinely multi-tenant.
	const t2 = "00000000-0000-0000-0000-0000000000f2"
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES ($1, 'other')`, t2); err != nil {
		t.Fatalf("insert non-default tenant: %v", err)
	}

	// Stepping the tenant migrations down must fail closed (0108's down gate
	// trips first on the non-default tenant_id, before 0106 is reached).
	src, err := iofs.New(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("iofs: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Steps(-countTenantMigrations(t)); err == nil {
		t.Fatal("stepping tenant migrations down must fail closed with a non-default tenant present")
	}
}

// countTenantMigrations returns how many embedded PostgreSQL migrations have a
// version >= 106 (the tenant-core migrations added by this work).
func countTenantMigrations(t *testing.T) int {
	t.Helper()
	entries, err := fs.ReadDir(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if len(name) < 5 || name[len(name)-7:] != ".up.sql" {
			continue
		}
		ver, err := strconv.Atoi(name[:4])
		if err != nil {
			continue
		}
		if ver >= 106 {
			n++
		}
	}
	return n
}

