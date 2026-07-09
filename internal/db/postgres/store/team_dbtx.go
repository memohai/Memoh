package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/teams"
)

const setLocalTeamSQL = "SELECT set_config('app.team_id', $1, true)"

type teamPoolDBTX struct {
	pool *pgxpool.Pool
}

func newTeamPoolDBTX(pool *pgxpool.Pool) *teamPoolDBTX {
	return &teamPoolDBTX{pool: pool}
}

func (db *teamPoolDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx, err := db.beginScopedTx(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	tag, err := tx.Exec(ctx, sql, args...)
	return tag, finishShortTx(ctx, tx, err)
}

func (db *teamPoolDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx, err := db.beginScopedTx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return &teamRows{ctx: ctx, tx: tx, Rows: rows}, nil
}

func (db *teamPoolDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	tx, err := db.beginScopedTx(ctx)
	if err != nil {
		return errRow{err: err}
	}
	return &teamRow{ctx: ctx, tx: tx, row: tx.QueryRow(ctx, sql, args...)}
}

func (db *teamPoolDBTX) beginScopedTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if err := setLocalTeam(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

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

func finishShortTx(ctx context.Context, tx pgx.Tx, err error) error {
	if err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

type teamRow struct {
	ctx context.Context
	tx  pgx.Tx
	row pgx.Row
}

func (r *teamRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	return finishShortTx(r.ctx, r.tx, err)
}

type errRow struct {
	err error
}

func (r errRow) Scan(...any) error {
	return r.err
}

type teamRows struct {
	ctx context.Context
	tx  pgx.Tx
	pgx.Rows
	closed bool
}

func (r *teamRows) Close() {
	if r.closed {
		return
	}
	r.closed = true
	r.Rows.Close()
	err := r.Err()
	if err != nil {
		_ = r.tx.Rollback(r.ctx)
		return
	}
	_ = r.tx.Commit(r.ctx)
}

func (r *teamRows) Next() bool {
	next := r.Rows.Next()
	if !next {
		r.Close()
	}
	return next
}
