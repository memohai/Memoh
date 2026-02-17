package modules

import (
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/server"
	"go.uber.org/fx"
)

var HandlersModule = fx.Module(
	"handlers",
	fx.Provide(
		// Custom handlers with provide functions
		annotateHandler(provideMemoryHandler),
		annotateHandler(provideAuthHandler),
		annotateHandler(provideMessageHandler),
		annotateHandler(provideUsersHandler),
		annotateHandler(provideCLIHandler),
		annotateHandler(provideWebHandler),

		// Simple handlers from handlers package
		annotateHandler(handlers.NewEmbeddingsHandler),
		annotateHandler(handlers.NewPingHandler),
		annotateHandler(handlers.NewSwaggerHandler),
		annotateHandler(handlers.NewProvidersHandler),
		annotateHandler(handlers.NewSearchProvidersHandler),
		annotateHandler(handlers.NewModelsHandler),
		annotateHandler(handlers.NewSettingsHandler),
		annotateHandler(handlers.NewPreauthHandler),
		annotateHandler(handlers.NewBindHandler),
		annotateHandler(handlers.NewScheduleHandler),
		annotateHandler(handlers.NewSubagentHandler),
		annotateHandler(handlers.NewChannelHandler),
		annotateHandler(handlers.NewMCPHandler),
	),
)

// annotateHandler wraps a handler provider function with fx.Annotate
// to register it as a server.Handler with the correct group tag
func annotateHandler(fn any) any {
	return fx.Annotate(
		fn,
		fx.As(new(server.Handler)),
		fx.ResultTags(`group:"server_handlers"`),
	)
}

// ---------------------------------------------------------------------------
// handler providers (interface adaptation / config extraction)
// ---------------------------------------------------------------------------

func provideMemoryHandler(log *slog.Logger, service *memory.Service, chatService *conversation.Service, accountService *accounts.Service, cfg config.Config, manager *mcp.Manager) *handlers.MemoryHandler {
	h := handlers.NewMemoryHandler(log, service, chatService, accountService)
	if manager != nil {
		execWorkDir := cfg.MCP.DataMount
		if strings.TrimSpace(execWorkDir) == "" {
			execWorkDir = config.DefaultDataMount
		}
		h.SetMemoryFS(memory.NewMemoryFS(log, manager, execWorkDir))
	}
	return h
}

func provideAuthHandler(log *slog.Logger, accountService *accounts.Service, rc *boot.RuntimeConfig) *handlers.AuthHandler {
	return handlers.NewAuthHandler(log, accountService, rc.JwtSecret, rc.JwtExpiresIn)
}

func provideMessageHandler(log *slog.Logger, resolver *flow.Resolver, chatService *conversation.Service, msgService *message.DBService, botService *bots.Service, accountService *accounts.Service, identityService *identities.Service, hub *event.Hub) *handlers.MessageHandler {
	return handlers.NewMessageHandler(log, resolver, chatService, msgService, botService, accountService, identityService, hub)
}

func provideUsersHandler(log *slog.Logger, accountService *accounts.Service, identityService *identities.Service, botService *bots.Service, routeService *route.DBService, channelService *channel.Service, channelManager *channel.Manager, registry *channel.Registry) *handlers.UsersHandler {
	return handlers.NewUsersHandler(log, accountService, identityService, botService, routeService, channelService, channelManager, registry)
}

func provideCLIHandler(channelManager *channel.Manager, channelService *channel.Service, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service) *handlers.LocalChannelHandler {
	return handlers.NewLocalChannelHandler(local.CLIType, channelManager, channelService, chatService, hub, botService, accountService)
}

func provideWebHandler(channelManager *channel.Manager, channelService *channel.Service, chatService *conversation.Service, hub *local.RouteHub, botService *bots.Service, accountService *accounts.Service) *handlers.LocalChannelHandler {
	return handlers.NewLocalChannelHandler(local.WebType, channelManager, channelService, chatService, hub, botService, accountService)
}
