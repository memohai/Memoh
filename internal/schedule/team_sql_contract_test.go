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
		if allTeamScheduleQueries[fields[0]] {
			// Intentionally all-team (process-wide bootstrap). Guard that it stays
			// that way: a team_id filter here would silently limit the startup
			// scheduler to the default team.
			if strings.Contains(block, "sqlc.arg(team_id)") {
				t.Errorf("%s query %s is all-team by design but filters team_id", relPath, fields[0])
			}
			continue
		}
		if !strings.Contains(block, "team_id") || !strings.Contains(block, "sqlc.arg(team_id)") {
			t.Errorf("%s query %s missing team_id scope", relPath, fields[0])
		}
	}
}

// allTeamScheduleQueries back the process-wide startup scheduler and must span
// all teams; each row carries team_id, which the scheduler threads into the job.
var allTeamScheduleQueries = map[string]bool{
	"ListEnabledSchedules": true,
}
