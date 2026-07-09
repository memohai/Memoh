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
}

func readEmbeddedTeamMigration(t *testing.T, path string) string {
	t.Helper()
	data, err := embeddeddb.MigrationsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	return string(data)
}
