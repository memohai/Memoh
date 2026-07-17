//go:build integration

package db_test

import (
	"context"
	"testing"
)

// TestMigrateLegacyInstallPreservesRows simulates upgrading an EXISTING
// (pre-team) install: it applies the migration chain up to the last
// pre-team migration, seeds representative business data, then applies the
// the consolidated team migration. It asserts no rows are lost, every row is
// backfilled to the default team, and the schema reached the final
// team-scoped shape. Existing installs upgrade in place without a wipe.
func TestMigrateLegacyInstallPreservesRows(t *testing.T) {
	ctx := context.Background()
	dsn := teamMigrationDSN(t)
	pool := resetToEmpty(t)

	teamSteps := countMigrationsFromTeamCore(t)

	// Apply the chain up to (but not including) the team migrations — the
	// "legacy install" state.
	stepUpToPreTeam(t, dsn, teamSteps)

	// teams must not exist yet (pre-team baseline).
	var teamsExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='teams')`).Scan(&teamsExists); err != nil {
		t.Fatalf("check teams pre-upgrade: %v", err)
	}
	if teamsExists {
		t.Fatal("teams must not exist before the team migrations")
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

	// Apply the team migrations (the upgrade).
	stepUp(t, dsn, teamSteps)

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

	// Every seeded row is backfilled to the default team.
	const defaultTeam = "00000000-0000-0000-0000-000000000001"
	for _, table := range []string{"users", "bots", "bot_sessions"} {
		var nonDefault int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM "+table+" WHERE team_id IS DISTINCT FROM $1", defaultTeam,
		).Scan(&nonDefault); err != nil {
			t.Fatalf("check backfill %s: %v", table, err)
		}
		if nonDefault != 0 {
			t.Errorf("%s has %d rows not backfilled to the default team", table, nonDefault)
		}
	}

	// The final schema keeps the existing PK and adds a team-prefixed unique
	// key that composite foreign keys can reference.
	var teamKeyCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		 WHERE con.contype='u' AND con.conrelid='public.bots'::regclass
		   AND con.conname LIKE 'memoh_team_key_%'
		   AND (SELECT a.attname FROM pg_attribute a
		         WHERE a.attrelid=con.conrelid AND a.attnum=con.conkey[1])='team_id'`).Scan(&teamKeyCount); err != nil {
		t.Fatalf("bots team key: %v", err)
	}
	if teamKeyCount != 1 {
		t.Errorf("after upgrade bots must have one team-prefixed key, got %d", teamKeyCount)
	}
}

// TestMigrateDownFailClosedForMultiTeam asserts that once a non-default team
// exists, stepping the team migrations down fails closed rather than
// destroying team data.
func TestMigrateDownFailClosedForMultiTeam(t *testing.T) {
	ctx := context.Background()
	dsn := teamMigrationDSN(t)
	pool := freshMigratedDB(t)

	const t2 = "00000000-0000-0000-0000-0000000000f2"
	if _, err := pool.Exec(ctx, `INSERT INTO teams (id, slug) VALUES ($1, 'other')`, t2); err != nil {
		t.Fatalf("insert non-default team: %v", err)
	}
	// Stepping the team migrations down must fail closed.
	if downErr := tryStepDown(t, dsn, countMigrationsFromTeamCore(t)); downErr == nil {
		t.Fatal("stepping team migrations down must fail closed with a non-default team present")
	}
}

// TestMigrateDownSingletonSafe asserts a clean singleton database can step the
// team migrations down and back up (reversibility on the supported down path).
func TestMigrateDownSingletonSafe(t *testing.T) {
	ctx := context.Background()
	dsn := teamMigrationDSN(t)
	pool := freshMigratedDB(t)

	steps := countMigrationsFromTeamCore(t)
	stepDown(t, dsn, steps)
	var teamsExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name='teams')`).Scan(&teamsExists); err != nil {
		t.Fatalf("check teams after down: %v", err)
	}
	if teamsExists {
		t.Error("clean singleton must be able to drop the team schema on down")
	}
	stepUp(t, dsn, steps)
}

func TestTeamMigrationsLeaveUserTablesUntouched(t *testing.T) {
	ctx := context.Background()
	dsn := teamMigrationDSN(t)
	pool := resetToEmpty(t)
	teamSteps := countMigrationsFromTeamCore(t)
	stepUpToPreTeam(t, dsn, teamSteps)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE public.user_extension_data (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			team_id uuid,
			payload text NOT NULL
		);
		INSERT INTO public.user_extension_data (team_id, payload)
		VALUES ('00000000-0000-0000-0000-0000000000ee', 'preserve me')`); err != nil {
		t.Fatalf("create user table: %v", err)
	}

	stepUp(t, dsn, teamSteps)

	var rls, forced bool
	if err := pool.QueryRow(ctx, `
		SELECT relrowsecurity, relforcerowsecurity
		  FROM pg_class WHERE oid='public.user_extension_data'::regclass`).Scan(&rls, &forced); err != nil {
		t.Fatalf("read user table RLS: %v", err)
	}
	if rls || forced {
		t.Errorf("team migrations changed user table RLS: enabled=%v forced=%v", rls, forced)
	}
	var teamFKs int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		 JOIN pg_class parent ON parent.oid=con.confrelid
		 WHERE con.conrelid='public.user_extension_data'::regclass
		   AND con.contype='f' AND parent.relname='teams'`).Scan(&teamFKs); err != nil {
		t.Fatalf("read user table FKs: %v", err)
	}
	if teamFKs != 0 {
		t.Errorf("team migrations added %d team FKs to user table", teamFKs)
	}
	var payload string
	if err := pool.QueryRow(ctx, `SELECT payload FROM public.user_extension_data`).Scan(&payload); err != nil {
		t.Fatalf("read user table row: %v", err)
	}
	if payload != "preserve me" {
		t.Errorf("user table payload = %q", payload)
	}
}
