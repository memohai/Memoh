// Package db provides PostgreSQL connection and pool helpers.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
)

// Open creates a pgx connection pool from the given Postgres config (DSN built from host, port, user, etc.).
func Open(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.SSLMode,
	)
	return pgxpool.New(ctx, dsn)
}
