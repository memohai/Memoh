package main

import (
	"log/slog"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	appchannel "github.com/memohai/memoh/internal/app/channel"
	appcore "github.com/memohai/memoh/internal/app/core"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/weixin"
	"github.com/memohai/memoh/internal/handlers"
)

func runServe() {
	fx.New(options()).Run()
}

func options() fx.Option {
	return fx.Options(
		fx.Provide(provideConfig),
		appcore.Module(),
		appchannel.Module(),
		fx.Provide(
			provideServerHandler(handlers.NewPingHandler),
			provideServerHandler(handlers.NewWebhookTunnelHandler),
			provideServerHandler(handlers.NewConfiguredPublicMediaHandler),
			provideServerHandler(provideAuthHandler),
			provideServerHandler(provideMemoryHandler),
			provideServerHandler(provideMessageHandler),
			provideServerHandler(provideSessionHandler),
			provideServerHandler(handlers.NewUserRuntimeHandler),
			provideServerHandler(handlers.NewRuntimeConnectHandler),
			provideServerHandler(handlers.NewBotRemoteRuntimeHandler),
			provideServerHandler(handlers.NewACPHandler),
			provideServerHandler(handlers.NewACPRuntimeHandler),
			provideServerHandler(handlers.NewSwaggerHandler),
			provideServerHandler(handlers.NewProvidersHandler),
			provideServerHandler(provideProviderOAuthHandler),
			provideServerHandler(provideACPCodexOAuthServerHandler),
			provideServerHandler(provideACPClaudeCodeOAuthServerHandler),
			provideServerHandler(handlers.NewFetchProvidersHandler),
			provideServerHandler(handlers.NewSearchProvidersHandler),
			provideServerHandler(handlers.NewModelsHandler),
			provideServerHandler(handlers.NewSettingsHandler),
			provideServerHandler(handlers.NewToolApprovalHandler),
			provideServerHandler(handlers.NewHooksHandler),
			provideServerHandler(handlers.NewACLHandler),
			provideServerHandler(handlers.NewBotUserAccessHandler),
			provideServerHandler(handlers.NewChannelAccessHandler),
			provideServerHandler(handlers.NewScheduleHandler),
			provideServerHandler(handlers.NewHeartbeatHandler),
			provideServerHandler(handlers.NewCompactionHandler),
			provideServerHandler(handlers.NewChannelHandler),
			provideServerHandler(channel.NewWebhookServerHandler),
			provideServerHandler(weixin.NewQRServerHandler),
			provideServerHandler(provideUsersHandler),
			provideServerHandler(handlers.NewMemoryProvidersHandler),
			provideServerHandler(handlers.NewNetworkHandler),
			provideServerHandler(handlers.NewAudioHandler),
			provideServerHandler(handlers.NewVideoHandler),
			provideServerHandler(handlers.NewBotAudioHandler),
			provideServerHandler(handlers.NewEmailProvidersHandler),
			provideServerHandler(handlers.NewEmailBindingsHandler),
			provideServerHandler(handlers.NewEmailOutboxHandler),
			provideServerHandler(handlers.NewEmailWebhookHandler),
			provideServerHandler(provideEmailOAuthHandler),
			provideServerHandler(handlers.NewMCPHandler),
			provideServerHandler(handlers.NewMCPOAuthHandler),
			provideServerHandler(handlers.NewPluginsHandler),
			provideServerHandler(handlers.NewBotBackupHandler),
			provideServerHandler(handlers.NewTokenUsageHandler),
			provideServerHandler(handlers.NewSessionInfoHandler),
			provideServerHandler(handlers.NewSupermarketHandler),
			provideServerHandler(provideWebHandler),
			provideServer,
		),
		fx.Invoke(
			startServer,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger.With(slog.String("component", "fx"))}
		}),
	)
}
