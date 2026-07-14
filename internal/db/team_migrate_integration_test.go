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
	"github.com/memohai/memoh/internal/team"
)

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

var (
	teamTestDBSeq atomic.Uint64
	teamTestDBs   sync.Map
)

// teamMigrationDSN creates a database dedicated to the current test. Team
// migration tests drop schemas and step migrations backward, so they must not
// share TEST_POSTGRES_DSN with other integration packages.
func teamMigrationDSN(t *testing.T) string {
	t.Helper()
	if dsn, ok := teamTestDBs.Load(t.Name()); ok {
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

	dbName := "memoh_team_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatUint(teamTestDBSeq.Add(1), 10)
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
	teamTestDBs.Store(t.Name(), testDSN)
	t.Cleanup(func() {
		teamTestDBs.Delete(t.Name())
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
	dsn := teamMigrationDSN(t)
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

func TestSingletonTeamSeededAfterMigrate(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM teams").Scan(&count); err != nil {
		t.Fatalf("count teams: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 seeded team, got %d", count)
	}

	var id, slug string
	if err := pool.QueryRow(ctx, "SELECT id::text, slug FROM teams").Scan(&id, &slug); err != nil {
		t.Fatalf("select team: %v", err)
	}
	if id != team.DefaultTeamID {
		t.Fatalf("seeded team id = %q, want DefaultTeamID %q", id, team.DefaultTeamID)
	}
	if slug != "default" {
		t.Fatalf("seeded team slug = %q, want %q", slug, "default")
	}

	// teams root must NOT carry a redundant team_id column (it is the root:
	// its own id IS the team id).
	var hasTeamID bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'teams' AND column_name = 'team_id'
		)`).Scan(&hasTeamID); err != nil {
		t.Fatalf("check teams columns: %v", err)
	}
	if hasTeamID {
		t.Fatal("teams root must not have a redundant team_id column")
	}
}

func TestMigrationsDoNotRequireClusterRolePrivileges(t *testing.T) {
	ctx := context.Background()
	adminDSN := teamMigrationDSN(t)
	adminCfg, err := pgconn.ParseConfig(adminDSN)
	if err != nil {
		t.Fatalf("parse isolated database DSN: %v", err)
	}
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect isolated database: %v", err)
	}

	role := "memoh_migration_test_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatUint(teamTestDBSeq.Add(1), 10)
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
	dsn := teamMigrationDSN(t)
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

// stepUpToPreTeam applies the migration chain up to (but not including) the
// teamSteps team migrations — i.e. the "legacy install" pre-team state.
func stepUpToPreTeam(t *testing.T, dsn string, teamSteps int) {
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
	if err := m.Steps(total - teamSteps); err != nil {
		t.Fatalf("step up to pre-team (%d steps): %v", total-teamSteps, err)
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

// TestTeamChainReversible verifies the full team migration chain
// is cleanly reversible: stepping down the consolidated team migration
// removes all team objects, and a step-up re-applies them. It also verifies
// the down safety gate refuses to drop the teams root when a non-default
// team exists.
func TestTeamChainReversible(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := teamMigrationDSN(t)

	// The team core is intentionally one consolidated migration.
	teamSteps := countTeamMigrations(t)

	// Step the team migrations down; teams + app schema must be gone.
	stepDown(t, dsn, teamSteps)
	var teamsExists, appExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='teams')`).Scan(&teamsExists); err != nil {
		t.Fatalf("check teams after step down: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name='app')`).Scan(&appExists); err != nil {
		t.Fatalf("check app schema after step down: %v", err)
	}
	if teamsExists {
		t.Error("teams root must be dropped after stepping down the team migrations")
	}
	if appExists {
		t.Error("app schema must be dropped after stepping down the team migrations")
	}

	// Re-apply and confirm SET NULL actions target only the original reference
	// column rather than the non-null team_id column.
	stepUp(t, dsn, teamSteps)
	var unsafeSetNull int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE con.contype='f' AND con.confdeltype='n' AND n.nspname='public'
		   AND (con.confdelsetcols IS NULL OR EXISTS (
		       SELECT 1 FROM pg_attribute a
		        WHERE a.attrelid=con.conrelid AND a.attnum=ANY(con.confdelsetcols)
		          AND a.attname='team_id'))`).Scan(&unsafeSetNull); err != nil {
		t.Fatalf("count set null after re-up: %v", err)
	}
	if unsafeSetNull != 0 {
		t.Errorf("after re-up found %d SET NULL FKs that can clear team_id", unsafeSetNull)
	}
}

// TestTeamsRootDownSafetyGate verifies the root down safety gate: when a
// non-default team exists, stepping the team migrations down must fail
// closed rather than dropping team data.
func TestTeamsRootDownSafetyGate(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := teamMigrationDSN(t)

	// Seed a second team so rollback must preserve the team root.
	const t2 = "00000000-0000-0000-0000-0000000000f2"
	if _, err := pool.Exec(ctx, `INSERT INTO teams (id, slug) VALUES ($1, 'other')`, t2); err != nil {
		t.Fatalf("insert non-default team: %v", err)
	}

	// Stepping the team migration down must fail closed.
	src, err := iofs.New(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("iofs: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Steps(-countTeamMigrations(t)); err == nil {
		t.Fatal("stepping team migrations down must fail closed with a non-default team present")
	}
}

// countTeamMigrations asserts that this PR contributes exactly one migration.
func countTeamMigrations(t *testing.T) int {
	t.Helper()
	entries, err := fs.ReadDir(postgresMigrationsFS(t), ".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	const teamMigration = "0107_team_core.up.sql"
	found := false
	for _, e := range entries {
		if e.Name() == teamMigration {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing consolidated team migration %s", teamMigration)
	}
	return 1
}
