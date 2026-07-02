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

func (q *Queries) WithTx(tx pgx.Tx) dbstore.Queries {
	if q == nil {
		return nil
	}
	return NewQueries(q.Queries.WithTx(tx))
}

func (q *Queries) RunInTx(ctx context.Context, fn func(dbstore.Queries) error) error {
	if q == nil {
		return nil
	}
	if q.pool == nil {
		return fn(q)
	}
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := fn(q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}
