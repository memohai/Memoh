package acl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChannelIdentityQueriesAreTeamScoped(t *testing.T) {
	sql := readQueryFile(t, "channel_identities.sql")

	requireSQLContains(t, sql, "team_id, channel_type, channel_subject_id")
	requireSQLContains(t, sql, "ON CONFLICT (team_id, channel_type, channel_subject_id)")
	requireSQLContains(t, sql, "WHERE team_id = sqlc.arg(team_id)")
}

func TestChannelAccessBindingQueriesAreTeamScoped(t *testing.T) {
	sql := readQueryFile(t, "channel_identity_bindings.sql")

	requireSQLContains(t, sql, "INSERT INTO channel_link_codes (token, team_id, user_id, channel_type, expires_at)")
	requireSQLContains(t, sql, "WHERE token = sqlc.arg(token)")
	requireSQLContains(t, sql, "AND team_id = sqlc.arg(team_id)")
	requireSQLContains(t, sql, "ci.team_id = b.team_id")
}

func TestBotScopedChannelAndACLQueriesKeepIdentityJoinsInTeam(t *testing.T) {
	for _, name := range []string{
		"bot_channel_admins.sql",
		"acl.sql",
	} {
		t.Run(name, func(t *testing.T) {
			sql := readQueryFile(t, name)
			requireSQLContains(t, sql, "ci.team_id =")
		})
	}
}

func readQueryFile(t *testing.T, name string) string {
	t.Helper()
	// #nosec G304 -- tests read fixed checked-in SQL query files.
	data, err := os.ReadFile(filepath.Join("..", "..", "db", "postgres", "queries", name))
	if err != nil {
		t.Fatalf("read query file %s: %v", name, err)
	}
	return string(data)
}

func requireSQLContains(t *testing.T, sql, needle string) {
	t.Helper()
	if !strings.Contains(sql, needle) {
		t.Fatalf("expected SQL to contain %q\nSQL:\n%s", needle, sql)
	}
}
