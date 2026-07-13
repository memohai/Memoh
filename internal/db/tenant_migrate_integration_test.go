//go:build integration

package db_test

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"testing"

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

	// Reset to a pristine schema so migrate up starts from empty.
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatalf("reset schema: %v", err)
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

// migrateSteps runs a migrate command (up/down with optional arg) against the DSN.
func migrateSteps(t *testing.T, dsn, command string, args []string) error {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return db.RunMigrate(logger, pgConfigFromDSN(t, dsn), postgresMigrationsFS(t), command, args)
}

// TestTenantsRootMigrationReversible verifies 0106 is cleanly reversible on a
// clean singleton database (up -> down one step -> up again), and that the down
// safety gate refuses to drop the root when a non-default tenant exists.
func TestTenantsRootMigrationReversible(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)

	// Roll back the single 0106 step (clean singleton): must succeed and drop tenants.
	if err := migrateSteps(t, dsn, "down", []string{"1"}); err != nil {
		t.Fatalf("down 1 on clean singleton: %v", err)
	}
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='tenants')`).Scan(&exists); err != nil {
		t.Fatalf("check tenants after down: %v", err)
	}
	if exists {
		t.Fatal("down 1 must drop tenants on a clean singleton database")
	}

	// Re-apply.
	if err := migrateSteps(t, dsn, "up", nil); err != nil {
		t.Fatalf("re-up: %v", err)
	}

	// Insert a non-default tenant, then attempt down: the safety gate must refuse.
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES (gen_random_uuid(), 'other')`); err != nil {
		t.Fatalf("insert non-default tenant: %v", err)
	}
	if err := migrateSteps(t, dsn, "down", []string{"1"}); err == nil {
		t.Fatal("down 1 must fail closed when a non-default tenant exists")
	}
}

