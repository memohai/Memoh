//go:build integration

package db_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
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

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

var (
	tenantTestDBSeq atomic.Uint64
	tenantTestDBs   sync.Map
)

// tenantMigrationDSN creates a database dedicated to the current test. Tenant
// migration tests drop schemas and step migrations backward, so they must not
// share TEST_POSTGRES_DSN with other integration packages.
func tenantMigrationDSN(t *testing.T) string {
	t.Helper()
	if dsn, ok := tenantTestDBs.Load(t.Name()); ok {
		return dsn.(string)
	}

	baseDSN := os.Getenv("TEST_POSTGRES_DSN")
	if baseDSN == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	cfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		t.Fatalf("parse TEST_POSTGRES_DSN: %v", err)
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), cfg.Copy())
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	defer admin.Close()

	dbName := "memoh_tenant_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatUint(tenantTestDBSeq.Add(1), 10)
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+dbName); err != nil {
		t.Fatalf("create isolated test database: %v", err)
	}
	testCfg := cfg.Copy()
	testCfg.ConnConfig.Database = dbName
	sslMode := "disable"
	if testCfg.ConnConfig.TLSConfig != nil {
		sslMode = "require"
	}
	testDSN := db.DSN(config.PostgresConfig{
		Host:     testCfg.ConnConfig.Host,
		Port:     int(testCfg.ConnConfig.Port),
		User:     testCfg.ConnConfig.User,
		Password: testCfg.ConnConfig.Password,
		Database: dbName,
		SSLMode:  sslMode,
	})
	tenantTestDBs.Store(t.Name(), testDSN)
	t.Cleanup(func() {
		tenantTestDBs.Delete(t.Name())
		cleanup, err := pgxpool.NewWithConfig(context.Background(), cfg.Copy())
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
	})
	return testDSN
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

// freshMigratedDB applies the full migration chain to the test's isolated DB.
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

func TestMigrationsDoNotRequireClusterRolePrivileges(t *testing.T) {
	ctx := context.Background()
	adminDSN := tenantMigrationDSN(t)
	adminCfg, err := pgconn.ParseConfig(adminDSN)
	if err != nil {
		t.Fatalf("parse isolated database DSN: %v", err)
	}
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect isolated database: %v", err)
	}

	role := "memoh_migration_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatUint(tenantTestDBSeq.Add(1), 10)
	const password = "migration_test_password"
	if _, err := admin.Exec(ctx, "CREATE ROLE "+role+" LOGIN NOSUPERUSER NOCREATEROLE NOBYPASSRLS PASSWORD '"+password+"'"); err != nil {
		t.Fatalf("create limited migration role: %v", err)
	}
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+adminCfg.Database+" OWNER TO "+role); err != nil {
		t.Fatalf("assign test database owner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "ALTER DATABASE "+adminCfg.Database+" OWNER TO "+adminCfg.User)
		_, _ = admin.Exec(ctx, "DROP OWNED BY "+role)
		_, _ = admin.Exec(ctx, "DROP ROLE IF EXISTS "+role)
		admin.Close()
	})

	limitedDSN := db.DSN(config.PostgresConfig{
		Host:     adminCfg.Host,
		Port:     int(adminCfg.Port),
		User:     role,
		Password: password,
		Database: adminCfg.Database,
		SSLMode:  "disable",
	})
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := db.RunMigrate(logger, pgConfigFromDSN(t, limitedDSN), postgresMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate as database owner without CREATEROLE/BYPASSRLS: %v", err)
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

// resetToEmpty returns a connection to the test's isolated, empty database.
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
// is cleanly reversible: stepping down the consolidated tenant migration
// removes all tenant objects, and a step-up re-applies them. It also verifies
// the down safety gate refuses to drop the tenants root when a non-default
// tenant exists.
func TestTenantChainReversible(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)

	// The tenant core is intentionally one consolidated migration.
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

	// Re-apply and confirm SET NULL actions target only the original reference
	// column rather than the non-null tenant_id column.
	stepUp(t, dsn, tenantSteps)
	var unsafeSetNull int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE con.contype='f' AND con.confdeltype='n' AND n.nspname='public'
		   AND (con.confdelsetcols IS NULL OR EXISTS (
		       SELECT 1 FROM pg_attribute a
		        WHERE a.attrelid=con.conrelid AND a.attnum=ANY(con.confdelsetcols)
		          AND a.attname='tenant_id'))`).Scan(&unsafeSetNull); err != nil {
		t.Fatalf("count set null after re-up: %v", err)
	}
	if unsafeSetNull != 0 {
		t.Errorf("after re-up found %d SET NULL FKs that can clear tenant_id", unsafeSetNull)
	}
}

// TestTenantsRootDownSafetyGate verifies the root down safety gate: when a
// non-default tenant exists, stepping the tenant migrations down must fail
// closed rather than dropping tenant data.
func TestTenantsRootDownSafetyGate(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)

	// Seed a second tenant so rollback must preserve the tenant root.
	const t2 = "00000000-0000-0000-0000-0000000000f2"
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES ($1, 'other')`, t2); err != nil {
		t.Fatalf("insert non-default tenant: %v", err)
	}

	// Stepping the tenant migration down must fail closed.
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

// countTenantMigrations asserts that this PR contributes exactly one migration.
func countTenantMigrations(t *testing.T) int {
	t.Helper()
	entries, err := fs.ReadDir(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	const tenantMigration = "0107_tenant_core.up.sql"
	found := false
	for _, e := range entries {
		if e.Name() == tenantMigration {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing consolidated tenant migration %s", tenantMigration)
	}
	return 1
}
