package teams

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestEnsureDefaultBootstrapsTeamAndMembers(t *testing.T) {
	db := &recordingBootstrapDB{}

	if err := EnsureDefault(context.Background(), db); err != nil {
		t.Fatalf("EnsureDefault returned error: %v", err)
	}
	if len(db.execs) != 2 {
		t.Fatalf("exec count = %d, want 2", len(db.execs))
	}
	if !strings.Contains(db.execs[0].sql, "INSERT INTO teams") {
		t.Fatalf("first exec should bootstrap teams, got %q", db.execs[0].sql)
	}
	if db.execs[0].args[0] != DefaultTeamID {
		t.Fatalf("default team id arg = %v", db.execs[0].args[0])
	}
	if !strings.Contains(db.execs[1].sql, "INSERT INTO team_members") {
		t.Fatalf("second exec should backfill team members, got %q", db.execs[1].sql)
	}
}

type recordingBootstrapDB struct {
	execs []recordedBootstrapExec
}

type recordedBootstrapExec struct {
	sql  string
	args []any
}

func (db *recordingBootstrapDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execs = append(db.execs, recordedBootstrapExec{sql: sql, args: args})
	return pgconn.CommandTag{}, nil
}
