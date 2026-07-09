package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginQueriesCarryTeamIsolation(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "postgres", "queries", "plugins.sql"))
	if err != nil {
		t.Fatalf("read plugins.sql: %v", err)
	}
	sql := string(raw)
	for _, needle := range []string{
		"team_id",
		"SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id)",
		"SELECT i.team_id FROM bot_plugin_installations i WHERE i.id = sqlc.arg(installation_id)",
		"ON CONFLICT (team_id, bot_id, plugin_id)",
		"ON CONFLICT (team_id, installation_id, resource_type, resource_key)",
	} {
		if !strings.Contains(sql, needle) {
			t.Fatalf("plugins.sql missing tenant isolation fragment %q", needle)
		}
	}
}
