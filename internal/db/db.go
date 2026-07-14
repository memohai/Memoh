package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/tenant"
)

func Open(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	switch driver := DriverFromConfig(cfg); driver {
	case DriverPostgres:
		return OpenPostgres(ctx, cfg.Postgres)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
}

func OpenPostgres(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(DSN(cfg))
	if err != nil {
		return nil, err
	}
	// Bind the singleton tenant on every new connection. Tenant business queries
	// scope themselves with app.current_tenant_id(), which fail-closed raises if
	// app.tenant_id is unset. Upstream is single-tenant, so we set the default
	// tenant at the session level here.
	poolCfg.AfterConnect = SetDefaultTenantOnConnect
	return pgxpool.NewWithConfig(ctx, poolCfg)
}

// SetDefaultTenantOnConnect is a pgxpool AfterConnect hook that binds the
// singleton tenant GUC (app.tenant_id) at the session level. It is exported so
// single-tenant test harnesses can install it on their own pools, matching the
// production connection setup.
func SetDefaultTenantOnConnect(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, "SELECT set_config('app.tenant_id', $1, false)", tenant.DefaultTenantID)
	return err
}

// OpenPostgresDSN opens a pool from a raw libpq DSN with the default-tenant
// AfterConnect hook installed. Single-tenant test harnesses use this so their
// pools bind the tenant GUC exactly like production.
func OpenPostgresDSN(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = SetDefaultTenantOnConnect
	return pgxpool.NewWithConfig(ctx, cfg)
}
