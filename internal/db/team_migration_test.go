package db

import (
	"strings"
	"testing"

	embeddeddb "github.com/memohai/memoh/db"
)

func TestTeamMultitenancyMigrationFiles(t *testing.T) {
	baseline := readEmbeddedTeamMigration(t, "postgres/migrations/0001_init.up.sql")
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS teams",
		"CREATE TABLE IF NOT EXISTS team_members",
		"team_id UUID",
		"app.team_id",
	} {
		if !strings.Contains(baseline, want) {
			t.Fatalf("0001_init.up.sql missing %q", want)
		}
	}

	incremental := readEmbeddedTeamMigration(t, "postgres/migrations/0105_team_multitenancy.up.sql")
	for _, want := range []string{
		"-- 0105_team_multitenancy",
		"CREATE TABLE IF NOT EXISTS teams",
		"CREATE TABLE IF NOT EXISTS team_members",
		"team_id",
	} {
		if !strings.Contains(incremental, want) {
			t.Fatalf("0105_team_multitenancy.up.sql missing %q", want)
		}
	}

	// 0106 drops users.role; verify the canonical schema no longer contains it.
	if strings.Contains(baseline, "user_role") {
		t.Fatal("0001_init.up.sql must not contain user_role enum after migration 0106")
	}
	if strings.Contains(baseline, "role user_role") {
		t.Fatal("0001_init.up.sql must not contain 'role user_role' column after migration 0106")
	}

	drop106 := readEmbeddedTeamMigration(t, "postgres/migrations/0106_drop_users_role.up.sql")
	for _, want := range []string{
		"-- 0106_drop_users_role",
		"DROP COLUMN IF EXISTS role",
		"DROP TYPE IF EXISTS user_role",
	} {
		if !strings.Contains(drop106, want) {
			t.Fatalf("0106_drop_users_role.up.sql missing %q", want)
		}
	}
}

func readEmbeddedTeamMigration(t *testing.T, path string) string {
	t.Helper()
	data, err := embeddeddb.MigrationsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	return string(data)
}
