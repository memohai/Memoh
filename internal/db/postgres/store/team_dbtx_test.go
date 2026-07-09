package postgresstore

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/teams"
)

// Both wrappers must satisfy the generated DBTX interface.
var (
	_ dbsqlc.DBTX = (*teamPoolDBTX)(nil)
	_ dbsqlc.DBTX = (*teamTxDBTX)(nil)
)

const teamID = "11111111-1111-1111-1111-111111111111"

func TestTeamTxDBTXExecSetsLocalTeamBeforeStatement(t *testing.T) {
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teamID})
	tx := &recordingTx{}
	db := newTeamTxDBTX(tx)

	if _, err := db.Exec(ctx, "SELECT $1::int", 42); err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	assertLocalTeamInjected(t, tx, "SELECT $1::int")
}

func TestTeamTxDBTXQuerySetsLocalTeamBeforeStatement(t *testing.T) {
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teamID})
	tx := &recordingTx{}
	db := newTeamTxDBTX(tx)

	rows, err := db.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	rows.Close()
	assertLocalTeamInjected(t, tx, "SELECT 1")
}

func TestTeamTxDBTXQueryRowSetsLocalTeamBeforeStatement(t *testing.T) {
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teamID})
	tx := &recordingTx{}
	db := newTeamTxDBTX(tx)

	db.QueryRow(ctx, "SELECT 1")
	assertLocalTeamInjected(t, tx, "SELECT 1")
}

// assertLocalTeamInjected verifies the transactional path runs the
// set_config('app.team_id', ...) statement immediately before the caller's
// statement, so a future FORCE ROW LEVEL SECURITY + non-owner role deployment
// keeps working.
func assertLocalTeamInjected(t *testing.T, tx *recordingTx, want string) {
	t.Helper()
	if len(tx.execs) != 2 {
		t.Fatalf("exec count = %d, want 2", len(tx.execs))
	}
	if tx.execs[0].sql != setLocalTeamSQL {
		t.Fatalf("first exec sql = %q, want %q", tx.execs[0].sql, setLocalTeamSQL)
	}
	if got := tx.execs[0].args[0]; got != teamID {
		t.Fatalf("team arg = %v, want %q", got, teamID)
	}
	if tx.execs[1].sql != want {
		t.Fatalf("second exec sql = %q, want %q", tx.execs[1].sql, want)
	}
}

// --- Pooled (non-transactional) path -----------------------------------------

func TestTeamPoolDBTXExecBatchesSetConfigBeforeStatement(t *testing.T) {
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teamID})
	pool := &recordingPool{}
	db := &teamPoolDBTX{pool: pool}

	if _, err := db.Exec(ctx, "UPDATE bots SET name = name", 1); err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	assertBatchedSetConfig(t, pool, "UPDATE bots SET name = name")
	assertConnReleased(t, pool)
}

func TestTeamPoolDBTXQueryBatchesSetConfigBeforeStatement(t *testing.T) {
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teamID})
	pool := &recordingPool{}
	db := &teamPoolDBTX{pool: pool}

	rows, err := db.Query(ctx, "SELECT id FROM bots")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	rows.Close()
	assertBatchedSetConfig(t, pool, "SELECT id FROM bots")
	assertConnReleased(t, pool)
}

func TestTeamPoolDBTXQueryRowBatchesSetConfigBeforeStatement(t *testing.T) {
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teamID})
	pool := &recordingPool{}
	db := &teamPoolDBTX{pool: pool}

	_ = db.QueryRow(ctx, "SELECT count(*) FROM bots").Scan(new(int))
	assertBatchedSetConfig(t, pool, "SELECT count(*) FROM bots")
	assertConnReleased(t, pool)
}

func TestTeamPoolDBTXUsesScopeTeam(t *testing.T) {
	const scopedTeam = "22222222-2222-2222-2222-222222222222"
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: scopedTeam})
	pool := &recordingPool{}
	db := &teamPoolDBTX{pool: pool}

	_ = db.QueryRow(ctx, "SELECT 1").Scan(new(int))
	if got := pool.conn.batch.QueuedQueries[0].Arguments[0]; got != scopedTeam {
		t.Fatalf("set_config team arg = %v, want %q", got, scopedTeam)
	}
}

func TestTeamPoolDBTXFallsBackToDefaultTeam(t *testing.T) {
	// No scope in context -> ScopeOrDefault yields the default team so
	// background paths still set app.team_id.
	pool := &recordingPool{}
	db := &teamPoolDBTX{pool: pool}

	_ = db.QueryRow(context.Background(), "SELECT 1").Scan(new(int))
	if got := pool.conn.batch.QueuedQueries[0].Arguments[0]; got != teams.DefaultTeamID {
		t.Fatalf("set_config team arg = %v, want default %q", got, teams.DefaultTeamID)
	}
}

// assertBatchedSetConfig verifies the pooled path pipelines
// set_config('app.team_id', <scope team>, false) ahead of the caller's
// statement in one batch on one connection.
func assertBatchedSetConfig(t *testing.T, pool *recordingPool, want string) {
	t.Helper()
	if pool.conn == nil {
		t.Fatal("no connection acquired")
	}
	b := pool.conn.batch
	if b == nil || len(b.QueuedQueries) != 2 {
		t.Fatalf("batch queued %d queries, want 2", queuedLen(b))
	}
	if b.QueuedQueries[0].SQL != setSessionTeamSQL {
		t.Fatalf("first batch sql = %q, want %q", b.QueuedQueries[0].SQL, setSessionTeamSQL)
	}
	if got := b.QueuedQueries[0].Arguments[0]; got != teamID {
		t.Fatalf("set_config team arg = %v, want %q", got, teamID)
	}
	if b.QueuedQueries[1].SQL != want {
		t.Fatalf("second batch sql = %q, want %q", b.QueuedQueries[1].SQL, want)
	}
}

func assertConnReleased(t *testing.T, pool *recordingPool) {
	t.Helper()
	if pool.conn == nil {
		t.Fatal("no connection acquired")
	}
	if pool.conn.released != 1 {
		t.Fatalf("conn released %d times, want exactly 1", pool.conn.released)
	}
	if !pool.conn.batchClosed {
		t.Fatal("batch results were not closed")
	}
}

func queuedLen(b *pgx.Batch) int {
	if b == nil {
		return 0
	}
	return len(b.QueuedQueries)
}

// recordingPool / recordingConn / recordingBatchResults capture the batched
// set_config + statement without a real database.

type recordingPool struct {
	conn *recordingConn
}

func (p *recordingPool) Acquire(context.Context) (teamConn, error) {
	p.conn = &recordingConn{}
	return p.conn, nil
}

type recordingConn struct {
	batch       *pgx.Batch
	released    int
	batchClosed bool
}

func (c *recordingConn) SendBatch(_ context.Context, b *pgx.Batch) pgx.BatchResults {
	c.batch = b
	return &recordingBatchResults{conn: c}
}

func (c *recordingConn) Release() { c.released++ }

type recordingBatchResults struct {
	conn *recordingConn
}

func (*recordingBatchResults) Exec() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, nil }
func (*recordingBatchResults) Query() (pgx.Rows, error)         { return noopRows{}, nil }
func (*recordingBatchResults) QueryRow() pgx.Row                { return noopRow{} }
func (r *recordingBatchResults) Close() error {
	r.conn.batchClosed = true
	return nil
}

type noopRow struct{}

func (noopRow) Scan(...any) error { return nil }

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

func (tx *recordingTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx.execs = append(tx.execs, recordedExec{sql: sql, args: args})
	return noopRows{}, nil
}

// noopRows is a minimal pgx.Rows stub so Query callers can Close without a nil
// dereference; the tests only assert the recorded SQL, not row contents.
type noopRows struct{}

func (noopRows) Close()                                       {}
func (noopRows) Err() error                                   { return nil }
func (noopRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (noopRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (noopRows) Next() bool                                   { return false }
func (noopRows) Scan(...any) error                            { return nil }
func (noopRows) Values() ([]any, error)                       { return nil, nil }
func (noopRows) RawValues() [][]byte                          { return nil }
func (noopRows) Conn() *pgx.Conn                              { return nil }

func (tx *recordingTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	tx.execs = append(tx.execs, recordedExec{sql: sql, args: args})
	return nil
}

func (*recordingTx) Conn() *pgx.Conn {
	panic("not used")
}
