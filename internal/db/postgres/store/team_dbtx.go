// Package postgresstore provides the pgx-backed implementation of the shared
// store.Queries interface, including the team-scoped DBTX wrappers used by the
// generated sqlc queries.
//
// Team isolation posture (current):
//
//   - Every generated query carries an explicit team_id predicate (see
//     team_scope.go), so callers already filter by the context team. That
//     predicate is the effective isolation mechanism today.
//   - Row-level security is ENABLEd on the team-scoped tables with a
//     team_isolation policy keyed on current_setting('app.team_id'), but the
//     policy is NOT enforced: the tables are not FORCE ROW LEVEL SECURITY and
//     the application connects as the table-owner role, which bypasses
//     non-forced RLS entirely. The policy therefore enforces nothing at
//     runtime.
//
// Because of that, the singleton (non-transactional) query path executes
// directly against the pool instead of wrapping each statement in its own
// transaction with a set_config('app.team_id', ...) call. The wrapping added
// three extra round trips per query (BEGIN + set_config + COMMIT) for a
// setting that only the currently-bypassed RLS policy reads, so it was pure
// overhead on an app-wide hot path.
//
// The set_config injection is retained on genuinely transactional paths
// (Begin/BeginTx flows via teamTxDBTX). There it is a single extra Exec inside
// an already-open transaction, so it is cheap, and it keeps a future
// FORCE ROW LEVEL SECURITY + dedicated (non-owner) application role deployment
// working without further changes.
//
// Follow-up for defense-in-depth: run the application under a dedicated,
// non-owner database role and add FORCE ROW LEVEL SECURITY on the team-scoped
// tables so the team_isolation policy becomes a hard backstop behind the
// explicit team_id predicates.
package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/teams"
)

const setLocalTeamSQL = "SELECT set_config('app.team_id', $1, true)"

// teamPoolDBTX is the non-transactional query path. It executes statements
// directly against the pool. Isolation is provided by the explicit team_id
// predicate baked into every generated query, so no per-statement transaction
// or set_config('app.team_id', ...) is needed (see the package doc comment).
type teamPoolDBTX struct {
	pool *pgxpool.Pool
}

func newTeamPoolDBTX(pool *pgxpool.Pool) *teamPoolDBTX {
	return &teamPoolDBTX{pool: pool}
}

func (db *teamPoolDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, args...)
}

func (db *teamPoolDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

func (db *teamPoolDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// teamTxDBTX is the transactional query path. It sets app.team_id (transaction
// local) before each statement so that a future FORCE ROW LEVEL SECURITY +
// non-owner application role deployment keeps working. The extra Exec runs
// inside the already-open transaction, so it does not add round trips beyond
// the transaction the caller already opened.
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
