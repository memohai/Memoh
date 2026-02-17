package modules

import (
	"context"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/conversation"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/mcp"
	mcpcontainer "github.com/memohai/memoh/internal/mcp/providers/container"
	mcpdirectory "github.com/memohai/memoh/internal/mcp/providers/directory"
	mcpmemory "github.com/memohai/memoh/internal/mcp/providers/memory"
	mcpmessage "github.com/memohai/memoh/internal/mcp/providers/message"
	mcpschedule "github.com/memohai/memoh/internal/mcp/providers/schedule"
	mcpweb "github.com/memohai/memoh/internal/mcp/providers/web"
	mcpfederation "github.com/memohai/memoh/internal/mcp/sources/federation"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
	"go.uber.org/fx"
)

var ContainerdModule = fx.Module(
	"containerd",
	fx.Provide(
		provideContainerdHandler,
		provideToolGatewayService,
	),
	fx.Invoke(startContainerReconciliation),
)

// ---------------------------------------------------------------------------
// containerd handler & tool gateway
// ---------------------------------------------------------------------------

func provideContainerdHandler(log *slog.Logger, service ctr.Service, cfg config.Config, botService *bots.Service, accountService *accounts.Service, policyService *policy.Service, queries *dbsqlc.Queries) *handlers.ContainerdHandler {
	return handlers.NewContainerdHandler(log, service, cfg.MCP, cfg.Containerd.Namespace, botService, accountService, policyService, queries)
}

func provideToolGatewayService(log *slog.Logger, cfg config.Config, channelManager *channel.Manager, registry *channel.Registry, channelService *channel.Service, scheduleService *schedule.Service, memoryService *memory.Service, chatService *conversation.Service, accountService *accounts.Service, settingsService *settings.Service, searchProviderService *searchproviders.Service, manager *mcp.Manager, containerdHandler *handlers.ContainerdHandler, mcpConnService *mcp.ConnectionService) *mcp.ToolGatewayService {
	messageExec := mcpmessage.NewExecutor(log, channelManager, channelManager, registry)
	directoryExec := mcpdirectory.NewExecutor(log, registry, channelService, registry)
	scheduleExec := mcpschedule.NewExecutor(log, scheduleService)
	memoryExec := mcpmemory.NewExecutor(log, memoryService, chatService, accountService)
	webExec := mcpweb.NewExecutor(log, settingsService, searchProviderService)
	execWorkDir := cfg.MCP.DataMount
	if strings.TrimSpace(execWorkDir) == "" {
		execWorkDir = config.DefaultDataMount
	}
	fsExec := mcpcontainer.NewExecutor(log, manager, execWorkDir)

	fedGateway := handlers.NewMCPFederationGateway(log, containerdHandler)
	fedSource := mcpfederation.NewSource(log, fedGateway, mcpConnService)

	svc := mcp.NewToolGatewayService(
		log,
		[]mcp.ToolExecutor{messageExec, directoryExec, scheduleExec, memoryExec, webExec, fsExec},
		[]mcp.ToolSource{fedSource},
	)
	containerdHandler.SetToolGatewayService(svc)
	return svc
}

func startContainerReconciliation(lc fx.Lifecycle, containerdHandler *handlers.ContainerdHandler, _ *mcp.ToolGatewayService) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go containerdHandler.ReconcileContainers(ctx)
			return nil
		},
	})
}
