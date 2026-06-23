package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"github.com/memohai/memoh/internal/config"
)

func Open(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	switch driver := DriverFromConfig(cfg); driver {
	case DriverPostgres:
		return OpenPostgres(ctx, cfg.Postgres)
	case DriverSQLite:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
}

func OpenPostgres(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(DSN(cfg))
	if err != nil {
		return nil, err
	}
	poolCfg.AfterConnect = pgxvec.RegisterTypes
	return pgxpool.NewWithConfig(ctx, poolCfg)
}
