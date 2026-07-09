package heartbeat

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHeartbeatQueriesRequireTeamScope(t *testing.T) {
	for _, relPath := range []string{
		"db/postgres/queries/heartbeat_logs.sql",
		"db/postgres/queries/bots.sql",
	} {
		requireTeamScopedSQL(t, relPath)
	}
}

func requireTeamScopedSQL(t *testing.T, relPath string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	// #nosec G304 -- tests read fixed checked-in SQL query files.
	data, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	for _, block := range strings.Split(string(data), "-- name: ")[1:] {
		fields := strings.Fields(block)
		if len(fields) == 0 {
			continue
		}
		if relPath == "db/postgres/queries/bots.sql" {
			// Only ListHeartbeatEnabledBots is relevant to heartbeat here, and it
			// is intentionally all-team (process-wide bootstrap): assert it does
			// NOT filter team_id, then skip the rest of bots.sql.
			if fields[0] == "ListHeartbeatEnabledBots" && strings.Contains(block, "sqlc.arg(team_id)") {
				t.Errorf("bots.sql query ListHeartbeatEnabledBots is all-team by design but filters team_id")
			}
			continue
		}
		if !strings.Contains(block, "team_id") || !strings.Contains(block, "sqlc.arg(team_id)") {
			t.Errorf("%s query %s missing team_id scope", relPath, fields[0])
		}
	}
}
