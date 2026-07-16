package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/team"
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
	// Bind the singleton team on every new connection. Team business queries
	// scope themselves with public.memoh_current_team_id(), which fail-closed raises if
	// memoh.team_id is unset. Upstream is single-team, so we set the default
	// team at the session level here.
	poolCfg.AfterConnect = SetDefaultTeamAndRoleOnConnect(cfg.RuntimeRole)
	return pgxpool.NewWithConfig(ctx, poolCfg)
}

// SetDefaultTeamAndRoleOnConnect switches application sessions to the
// configured non-superuser role before binding the singleton team.
func SetDefaultTeamAndRoleOnConnect(runtimeRole string) func(context.Context, *pgx.Conn) error {
	return func(ctx context.Context, conn *pgx.Conn) error {
		if role := strings.TrimSpace(runtimeRole); role != "" {
			if _, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
				return fmt.Errorf("set postgres runtime role %q: %w", role, err)
			}
		}
		return SetDefaultTeamOnConnect(ctx, conn)
	}
}

// SetDefaultTeamOnConnect is a pgxpool AfterConnect hook that binds the
// singleton team GUC (memoh.team_id) at the session level. It is exported so
// single-team test harnesses can install it on their own pools, matching the
// production connection setup.
func SetDefaultTeamOnConnect(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, "SELECT set_config('memoh.team_id', $1, false)", team.DefaultTeamID)
	return err
}

// OpenPostgresDSN opens a pool from a raw libpq DSN with the default-team
// AfterConnect hook installed. Single-team test harnesses use this so their
// pools bind the team GUC exactly like production.
func OpenPostgresDSN(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = SetDefaultTeamOnConnect
	return pgxpool.NewWithConfig(ctx, cfg)
}
