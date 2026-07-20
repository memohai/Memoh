// cmd/channel is the Channel boundary's single-instance verification
// binary (spec §7.3): it assembles the shared Channel module over the
// in-process turn adapter, hosting only the platform webhook endpoints.
// Until a cross-process turn transport exists it is functionally an
// all-in-one without the REST API — its purpose is proving the channel
// dependency set stays explicit and closed, not serving deployments.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	channelmodule "github.com/memohai/memoh/cmd/internal/channel"
	coremodule "github.com/memohai/memoh/cmd/internal/core"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/weixin"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/server"
	"github.com/memohai/memoh/internal/version"
)

func main() {
	fx.New(options()).Run()
}

func provideConfig() (config.Config, error) {
	cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func provideServerHandler(fn any) any {
	return fx.Annotate(
		fn,
		fx.As(new(server.Handler)),
		fx.ResultTags(`group:"server_handlers"`),
	)
}

type serverParams struct {
	fx.In

	Logger         *slog.Logger
	RuntimeConfig  *boot.RuntimeConfig
	Config         config.Config
	ServerHandlers []server.Handler `group:"server_handlers"`
}

// provideServer hosts only the channel-owned HTTP surface: platform
// webhooks and the weixin QR callback, plus ping for liveness.
func provideServer(params serverParams) *server.Server {
	return server.NewServer(params.Logger, params.RuntimeConfig.ServerAddr, params.Config.Auth.JWTSecret, params.ServerHandlers...)
}

func startServer(lc fx.Lifecycle, logger *slog.Logger, srv *server.Server, shutdowner fx.Shutdowner) {
	fmt.Printf("Starting Memoh Channel %s\n", version.GetInfo())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error("server failed", slog.Any("error", err))
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Stop(ctx)
		},
	})
}

func options() fx.Option {
	return fx.Options(
		fx.Provide(provideConfig),
		coremodule.Module(),
		channelmodule.Module(),
		fx.Provide(
			provideServerHandler(handlers.NewPingHandler),
			provideServerHandler(channel.NewWebhookServerHandler),
			provideServerHandler(weixin.NewQRServerHandler),
			provideServer,
		),
		fx.Invoke(startServer),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger.With(slog.String("component", "fx"))}
		}),
	)
}
