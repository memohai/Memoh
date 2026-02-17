package modules

import (
	"context"
	"log/slog"
	"time"

	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/feishu"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/adapters/telegram"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/inbound"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/preauth"
	"go.uber.org/fx"
)

var ChannelModule = fx.Module(
    "channel",
    fx.Provide(
        local.NewRouteHub,
        channel.NewService,
        provideChannelRegistry,
        provideChannelRouter,
        provideChannelManager,
    ),
    fx.Invoke(startChannelManager),
)

// ---------------------------------------------------------------------------
// channel providers
// ---------------------------------------------------------------------------

func provideChannelRegistry(log *slog.Logger, hub *local.RouteHub) *channel.Registry {
	registry := channel.NewRegistry()
	registry.MustRegister(telegram.NewTelegramAdapter(log))
	registry.MustRegister(feishu.NewFeishuAdapter(log))
	registry.MustRegister(local.NewCLIAdapter(hub))
	registry.MustRegister(local.NewWebAdapter(hub))
	return registry
}

func provideChannelRouter(log *slog.Logger, registry *channel.Registry, routeService *route.DBService, msgService *message.DBService, resolver *flow.Resolver, identityService *identities.Service, botService *bots.Service, policyService *policy.Service, preauthService *preauth.Service, bindService *bind.Service, rc *boot.RuntimeConfig) *inbound.ChannelInboundProcessor {
	return inbound.NewChannelInboundProcessor(log, registry, routeService, msgService, resolver, identityService, botService, policyService, preauthService, bindService, rc.JwtSecret, 5*time.Minute)
}

func provideChannelManager(log *slog.Logger, registry *channel.Registry, channelService *channel.Service, channelRouter *inbound.ChannelInboundProcessor) *channel.Manager {
	mgr := channel.NewManager(log, registry, channelService, channelRouter)
	if mw := channelRouter.IdentityMiddleware(); mw != nil {
		mgr.Use(mw)
	}
	return mgr
}

func startChannelManager(lc fx.Lifecycle, channelManager *channel.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			channelManager.Start(ctx)
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			return channelManager.Shutdown(stopCtx)
		},
	})
}

