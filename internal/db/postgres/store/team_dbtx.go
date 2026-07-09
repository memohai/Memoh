// Package postgresstore provides the pgx-backed implementation of the shared
// store.Queries interface, including the team-scoped DBTX wrappers used by the
// generated sqlc queries.
//
// Team isolation posture:
//
//   - Every generated query carries an explicit team_id predicate (see
//     team_scope.go), so callers already filter by the context team.
//   - Row-level security is ENABLEd AND (migration 0107) FORCEd on the
//     team-scoped tables with a team_isolation policy keyed on
//     current_setting('app.team_id'). The runtime connects as the non-owner,
//     non-superuser role memoh_app, so FORCE RLS is a hard database backstop
//     behind the explicit team_id predicates.
//
// Because FORCE RLS is enforced, every application statement must have
// app.team_id set on the same connection that runs it. The singleton
// (non-transactional) query path therefore pipelines a
// set_config('app.team_id', <teamID>, false) call together with the real
// statement in a single round trip via pgx SendBatch, on one pooled
// connection. The team comes from teams.ScopeOrDefault(ctx) so detached
// background goroutines (which carry Scope in their context) and system paths
// both resolve a team.
//
// Invariant: every app query goes through teamPoolDBTX (or teamTxDBTX) and sets
// app.team_id before the statement runs, so a stale value left on a pooled
// connection is always overwritten before it is read; no extra reset round trip
// is needed. set_config uses is_local = false (session scope) on the pool path
// because there is no surrounding transaction; the next checkout overwrites it.
//
// The transactional query path (teamTxDBTX) keeps the transaction-local
// SET LOCAL app.team_id it already used.
package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/teams"
)

// setSessionTeamSQL sets app.team_id at session scope (is_local = false). Used
// on the pooled (non-transactional) path where there is no surrounding
// transaction; it is overwritten on the next checkout of any pooled connection.
const setSessionTeamSQL = "SELECT set_config('app.team_id', $1, false)"

// setLocalTeamSQL sets app.team_id transaction-local (is_local = true). Used on
// the transactional path so the value is scoped to that transaction.
const setLocalTeamSQL = "SELECT set_config('app.team_id', $1, true)"

// teamConn is the subset of *pgxpool.Conn the pooled path needs. Kept as an
// interface so tests can inject a recording connection.
type teamConn interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	Release()
}

// teamPool is the subset of *pgxpool.Pool the pooled path needs.
type teamPool interface {
	Acquire(ctx context.Context) (teamConn, error)
}

// pgxPoolAdapter adapts a *pgxpool.Pool to teamPool (Acquire returns the
// concrete *pgxpool.Conn, which satisfies teamConn).
type pgxPoolAdapter struct {
	pool *pgxpool.Pool
}

func (p pgxPoolAdapter) Acquire(ctx context.Context) (teamConn, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// teamPoolDBTX is the non-transactional query path. For every statement it
// acquires a pooled connection and pipelines set_config('app.team_id', ...)
// with the real statement in a single round trip via SendBatch, so FORCE RLS
// sees the correct team on the connection that runs the statement.
type teamPoolDBTX struct {
	pool teamPool
}

func newTeamPoolDBTX(pool *pgxpool.Pool) *teamPoolDBTX {
	return &teamPoolDBTX{pool: pgxPoolAdapter{pool: pool}}
}

// batchTeam builds a batch that first sets app.team_id (session scope) then
// queues the real statement, and sends it on conn. The caller is responsible
// for reading the first (set_config) result, reading its real result, and
// finally closing the returned BatchResults and releasing conn.
func batchTeam(ctx context.Context, conn teamConn, teamID, sql string, args ...any) pgx.BatchResults {
	batch := &pgx.Batch{}
	batch.Queue(setSessionTeamSQL, teamID)
	batch.Queue(sql, args...)
	return conn.SendBatch(ctx, batch)
}

func (db *teamPoolDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer conn.Release()

	br := batchTeam(ctx, conn, teams.ScopeOrDefault(ctx).TeamID, sql, args...)
	// Read and discard the set_config result before the real statement.
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		return pgconn.CommandTag{}, err
	}
	tag, execErr := br.Exec()
	closeErr := br.Close()
	if execErr != nil {
		return pgconn.CommandTag{}, execErr
	}
	if closeErr != nil {
		return pgconn.CommandTag{}, closeErr
	}
	return tag, nil
}

func (db *teamPoolDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return errRows{err: err}, err
	}

	br := batchTeam(ctx, conn, teams.ScopeOrDefault(ctx).TeamID, sql, args...)
	// Read and discard the set_config result before the real statement.
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		conn.Release()
		return errRows{err: err}, err
	}
	rows, err := br.Query()
	if err != nil {
		_ = br.Close()
		conn.Release()
		return errRows{err: err}, err
	}
	// The connection and batch must stay alive until the caller closes rows.
	return &batchPoolRows{Rows: rows, br: br, conn: conn}, nil
}

func (db *teamPoolDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return errRow{err: err}
	}

	br := batchTeam(ctx, conn, teams.ScopeOrDefault(ctx).TeamID, sql, args...)
	// Read and discard the set_config result before the real statement.
	if _, err := br.Exec(); err != nil {
		_ = br.Close()
		conn.Release()
		return errRow{err: err}
	}
	row := br.QueryRow()
	// The connection and batch must stay alive until Scan runs.
	return &batchPoolRow{Row: row, br: br, conn: conn}
}

// batchPoolRows wraps the real statement's rows from a batch so that closing
// the rows also closes the batch results and releases the pooled connection.
// Next/Scan mirror pgxpool.poolRows so the connection is released as soon as
// iteration completes.
type batchPoolRows struct {
	pgx.Rows
	br     pgx.BatchResults
	conn   teamConn
	closed bool
}

func (r *batchPoolRows) Close() {
	if r.closed {
		return
	}
	r.closed = true
	r.Rows.Close()
	_ = r.br.Close()
	r.conn.Release()
}

func (r *batchPoolRows) Next() bool {
	n := r.Rows.Next()
	if !n {
		r.Close()
	}
	return n
}

func (r *batchPoolRows) Scan(dest ...any) error {
	err := r.Rows.Scan(dest...)
	if err != nil {
		r.Close()
	}
	return err
}

// batchPoolRow wraps the real statement's row from a batch so that scanning it
// closes the batch results and releases the pooled connection, mirroring
// pgxpool.poolRow.
type batchPoolRow struct {
	pgx.Row
	br   pgx.BatchResults
	conn teamConn
}

func (r *batchPoolRow) Scan(dest ...any) error {
	panicked := true
	defer func() {
		if panicked {
			_ = r.br.Close()
			r.conn.Release()
		}
	}()
	err := r.Row.Scan(dest...)
	panicked = false
	_ = r.br.Close()
	r.conn.Release()
	return err
}

// teamTxDBTX is the transactional query path. It sets app.team_id (transaction
// local) before each statement so FORCE ROW LEVEL SECURITY under the non-owner
// runtime role keeps working. The extra Exec runs inside the already-open
// transaction, so it does not add round trips beyond the transaction the caller
// already opened.
type teamTxDBTX struct {
	tx pgx.Tx
}

func newTeamTxDBTX(tx pgx.Tx) *teamTxDBTX {
	return &teamTxDBTX{tx: tx}
}

func (db *teamTxDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if err := setLocalTeam(ctx, db.tx); err != nil {
		return pgconn.CommandTag{}, err
	}
	return db.tx.Exec(ctx, sql, args...)
}

func (db *teamTxDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if err := setLocalTeam(ctx, db.tx); err != nil {
		return nil, err
	}
	return db.tx.Query(ctx, sql, args...)
}

func (db *teamTxDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if err := setLocalTeam(ctx, db.tx); err != nil {
		return errRow{err: err}
	}
	return db.tx.QueryRow(ctx, sql, args...)
}

func setLocalTeam(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, setLocalTeamSQL, teams.ScopeOrDefault(ctx).TeamID)
	return err
}

type errRow struct {
	err error
}

func (r errRow) Scan(...any) error {
	return r.err
}

// errRows is a pgx.Rows that only reports an error, used when the pooled path
// fails before it can return real rows.
type errRows struct {
	err error
}

func (errRows) Close()                                       {}
func (e errRows) Err() error                                 { return e.err }
func (errRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (errRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (errRows) Next() bool                                   { return false }
func (e errRows) Scan(...any) error                          { return e.err }
func (e errRows) Values() ([]any, error)                     { return nil, e.err }
func (errRows) RawValues() [][]byte                          { return nil }
func (errRows) Conn() *pgx.Conn                              { return nil }
