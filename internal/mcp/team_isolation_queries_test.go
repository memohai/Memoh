package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPQueriesCarryTeamIsolation(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"mcp.sql", "mcp_oauth.sql"} {
		// #nosec G304 -- tests read fixed checked-in SQL query files.
		raw, err := os.ReadFile(filepath.Join("..", "..", "db", "postgres", "queries", file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		sql := string(raw)
		if !strings.Contains(sql, "team_id") {
			t.Fatalf("%s does not reference team_id", file)
		}
		if !strings.Contains(sql, "SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id)") &&
			!strings.Contains(sql, "SELECT c.team_id FROM mcp_connections c WHERE c.id = sqlc.arg(connection_id)") &&
			!strings.Contains(sql, "c.team_id = t.team_id") {
			t.Fatalf("%s does not scope lookups through the owning tenant", file)
		}
	}
}
