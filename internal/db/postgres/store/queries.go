package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type Queries struct {
	*dbsqlc.Queries
	pool *pgxpool.Pool
}

func NewQueries(queries *dbsqlc.Queries) *Queries {
	return &Queries{Queries: queries}
}

func NewQueriesWithPool(pool *pgxpool.Pool, queries *dbsqlc.Queries) *Queries {
	return &Queries{Queries: queries, pool: pool}
}

func (*Queries) SupportsAtomicDirectHistoryTurnWrites() bool {
	return true
}

// SupportsTransactions reports whether InTx opens a real PostgreSQL
// transaction. The pool-less wrapper intentionally retains its historical
// direct-execution fallback for tests and legacy callers.
func (q *Queries) SupportsTransactions() bool {
	return q != nil && q.pool != nil
}

func (q *Queries) WithTx(tx pgx.Tx) dbstore.Queries {
	if q == nil {
		return nil
	}
	return NewQueries(q.Queries.WithTx(tx))
}

func (q *Queries) InTx(ctx context.Context, fn func(dbstore.Queries) error) error {
	if q == nil || q.pool == nil {
		return fn(q)
	}
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return dbstore.MarkPersistenceRetrySafe(err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := fn(q.WithTx(tx)); err != nil {
		return dbstore.MarkPersistenceRetrySafe(err)
	}
	return tx.Commit(ctx)
}
