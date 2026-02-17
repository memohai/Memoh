package modules

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
	"go.uber.org/fx"
)

var InfraModule = fx.Module(
    "Infra",
    fx.Provide(
        provideConfig,
        provideLogger,
        provideContainerdClient,
        provideDBConn,
        provideDBQueries,
        provideMCPManager,
        boot.ProvideRuntimeConfig,
	    fx.Annotate(ctr.NewDefaultService, fx.As(new(ctr.Service))),
    ),
)

// ---------------------------------------------------------------------------
// infrastructure providers
// ---------------------------------------------------------------------------

func provideConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func provideLogger(cfg config.Config) *slog.Logger {
	logger.Init(cfg.Log.Level, cfg.Log.Format)
	return logger.L
}

func provideContainerdClient(lc fx.Lifecycle, rc *boot.RuntimeConfig) (*containerd.Client, error) {
	factory := ctr.DefaultClientFactory{SocketPath: rc.ContainerdSocketPath}
	client, err := factory.New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("connect containerd: %w", err)
	}
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return client.Close()
		},
	})
	return client, nil
}

func provideDBConn(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	conn, err := db.Open(context.Background(), cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			conn.Close()
			return nil
		},
	})
	return conn, nil
}

func provideDBQueries(conn *pgxpool.Pool) *dbsqlc.Queries {
	return dbsqlc.New(conn)
}

func provideMCPManager(log *slog.Logger, service ctr.Service, cfg config.Config, conn *pgxpool.Pool) *mcp.Manager {
	return mcp.NewManager(log, service, cfg.MCP, cfg.Containerd.Namespace, conn)
}