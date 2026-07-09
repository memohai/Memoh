package postgresstore

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/memohai/memoh/internal/teams"
)

func TestTeamTxDBTXExecSetsLocalTeamBeforeStatement(t *testing.T) {
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: "11111111-1111-1111-1111-111111111111"})
	tx := &recordingTx{}
	db := newTeamTxDBTX(tx)

	if _, err := db.Exec(ctx, "SELECT $1::int", 42); err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if len(tx.execs) != 2 {
		t.Fatalf("exec count = %d, want 2", len(tx.execs))
	}
	if tx.execs[0].sql != "SELECT set_config('app.team_id', $1, true)" {
		t.Fatalf("first exec sql = %q", tx.execs[0].sql)
	}
	if got := tx.execs[0].args[0]; got != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("team arg = %v", got)
	}
	if tx.execs[1].sql != "SELECT $1::int" {
		t.Fatalf("second exec sql = %q", tx.execs[1].sql)
	}
}

type recordingTx struct {
	execs []recordedExec
}

type recordedExec struct {
	sql  string
	args []any
}

func (*recordingTx) Begin(context.Context) (pgx.Tx, error) {
	panic("not used")
}

func (*recordingTx) Commit(context.Context) error {
	panic("not used")
}

func (*recordingTx) Rollback(context.Context) error {
	panic("not used")
}

func (*recordingTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("not used")
}

func (*recordingTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("not used")
}

func (*recordingTx) LargeObjects() pgx.LargeObjects {
	panic("not used")
}

func (*recordingTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("not used")
}

func (tx *recordingTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.execs = append(tx.execs, recordedExec{sql: sql, args: args})
	return pgconn.CommandTag{}, nil
}

func (*recordingTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("not used")
}

func (*recordingTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("not used")
}

func (*recordingTx) Conn() *pgx.Conn {
	panic("not used")
}
