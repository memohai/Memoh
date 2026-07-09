package workspace

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimePersistenceQueriesAreTeamScoped(t *testing.T) {
	files := map[string][]string{
		"../../db/postgres/queries/containers.sql": {
			"sqlc.arg(team_id)",
			"team_id",
		},
		"../../db/postgres/queries/versions.sql": {
			"sqlc.arg(team_id)",
			"containers",
			"team_id",
		},
		"../../db/postgres/queries/snapshots.sql": {
			"sqlc.arg(team_id)",
			"containers",
			"team_id",
		},
		"../../db/postgres/queries/events.sql": {
			"sqlc.arg(team_id)",
			"containers",
			"team_id",
		},
		"../../db/postgres/queries/workspace_resource_limits.sql": {
			"sqlc.arg(team_id)",
			"bots",
			"team_id",
		},
	}

	for file, required := range files {
		// #nosec G304 -- tests read fixed checked-in SQL query files.
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		sql := strings.ToLower(string(body))
		for _, token := range required {
			if !strings.Contains(sql, strings.ToLower(token)) {
				t.Fatalf("%s is missing team-scope token %q", file, token)
			}
		}
	}
}
