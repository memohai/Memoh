package schedule

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestScheduleQueriesRequireTeamScope(t *testing.T) {
	for _, relPath := range []string{
		"db/postgres/queries/schedule.sql",
		"db/postgres/queries/schedule_logs.sql",
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
		if !strings.Contains(block, "team_id") || !strings.Contains(block, "sqlc.arg(team_id)") {
			t.Errorf("%s query %s missing team_id scope", relPath, fields[0])
		}
	}
}
