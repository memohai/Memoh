package postgresstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type Queries struct {
	*dbsqlc.Queries
	pool     *pgxpool.Pool
	lockPool *pgxpool.Pool
	conn     *pgxpool.Conn
}

func NewQueries(queries *dbsqlc.Queries) *Queries {
	return &Queries{Queries: queries}
}

func NewQueriesWithPool(pool *pgxpool.Pool, queries *dbsqlc.Queries) *Queries {
	return NewQueriesWithPools(pool, nil, queries)
}

func NewQueriesWithPools(pool, lockPool *pgxpool.Pool, queries *dbsqlc.Queries) *Queries {
	return &Queries{Queries: queries, pool: pool, lockPool: lockPool}
}

func newQueriesWithConn(conn *pgxpool.Conn) *Queries {
	return &Queries{Queries: dbsqlc.New(conn), conn: conn}
}

func (*Queries) SupportsAtomicDirectHistoryTurnWrites() bool {
	return true
}

// SupportsTransactions reports whether InTx opens a real PostgreSQL
// transaction. Wrappers without a pool or pinned connection retain the
// historical direct-execution fallback for tests and legacy callers.
func (q *Queries) SupportsTransactions() bool {
	return q != nil && (q.pool != nil || q.conn != nil)
}

func (q *Queries) WithTx(tx pgx.Tx) dbstore.Queries {
	if q == nil {
		return nil
	}
	return NewQueries(q.Queries.WithTx(tx))
}

func (q *Queries) InTx(ctx context.Context, fn func(dbstore.Queries) error) error {
	if q == nil || (q.pool == nil && q.conn == nil) {
		return fn(q)
	}
	var (
		tx  pgx.Tx
		err error
	)
	if q.conn != nil {
		tx, err = q.conn.Begin(ctx)
	} else {
		tx, err = q.pool.Begin(ctx)
	}
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := fn(q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithBotMutationLock serializes workspace ownership mutations for one bot
// across all processes sharing this PostgreSQL database. The callback receives
// queries pinned to the lock-owning connection so nested transactions do not
// need a second pool connection.
func (q *Queries) WithBotMutationLock(
	ctx context.Context,
	botID pgtype.UUID,
	fn func(dbstore.Queries) error,
) (err error) {
	if q == nil || q.lockPool == nil {
		return fn(q)
	}
	if !botID.Valid {
		return errors.New("bot id is invalid")
	}

	conn, err := q.lockPool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire bot mutation lock connection: %w", err)
	}
	if _, err = conn.Exec(
		ctx,
		"SELECT pg_advisory_lock(hashtextextended($1, 0))",
		botMutationLockKey(botID),
	); err != nil {
		raw := conn.Hijack()
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return errors.Join(fmt.Errorf("acquire bot mutation lock: %w", err), raw.Close(closeCtx))
	}

	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		var unlocked bool
		unlockErr := conn.QueryRow(
			unlockCtx,
			"SELECT pg_advisory_unlock(hashtextextended($1, 0))",
			botMutationLockKey(botID),
		).Scan(&unlocked)
		cancel()
		if unlockErr != nil || !unlocked {
			raw := conn.Hijack()
			closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			closeErr := raw.Close(closeCtx)
			closeCancel()
			if unlockErr == nil {
				unlockErr = errors.New("PostgreSQL did not release the bot mutation lock")
			}
			slog.Default().Error(
				"failed to release bot mutation lock",
				slog.String("bot_id", botID.String()),
				slog.Any("error", errors.Join(unlockErr, closeErr)),
			)
			return
		}
		conn.Release()
	}()
	return fn(newQueriesWithConn(conn))
}

func botMutationLockKey(botID pgtype.UUID) string {
	return "memoh:bot-resource-mutation:" + botID.String()
}
