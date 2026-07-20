package core

import (
	"go.uber.org/fx"

	"github.com/memohai/memoh/internal/acl"
	audiopkg "github.com/memohai/memoh/internal/audio"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channelaccess"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/fetchproviders"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/mcp"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthclients"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
	"github.com/memohai/memoh/internal/userruntime"
	videopkg "github.com/memohai/memoh/internal/video"
	"github.com/memohai/memoh/internal/workspace"
)

// Module assembles the shared domain and agent-runtime providers used by
// the Memoh binaries. It expects config.Config to be provided by the
// composing command.
func Module() fx.Option {
	return fx.Options(
		fx.Provide(
			boot.ProvideRuntimeConfig,
			provideLogger,
			provideContainerService,
			provideOverlayProviderRegistry,
			provideNetworkService,
			provideNetworkController,
			provideDBConn,
			providePGVectorStore,
			providePostgresStore,
			provideDBQueries,
			provideAccountStore,
			provideUserRuntimeStore,
			provideBotRemoteRuntimeBindingStore,
			provideUserRuntimeHub,
			userruntime.NewService,
			workspace.NewRemoteWorkspaceService,
			provideUserRuntimePipe,
			provideWikiStore,
			provideWorkspaceManager,
			provideBridgeProvider,
			providePluginBridgeProvider,
			provideMemoryLLM,
			memprovider.NewService,
			provideMemoryProviderRegistry,
			models.NewService,
			bots.NewService,
			provideACPRunner,
			provideACPSessionPool,
			provideACPCodexOAuthHandler,
			provideACPClaudeCodeOAuthHandler,
			provideAccountService,
			acl.NewService,
			channelaccess.NewService,
			settings.NewService,
			toolapproval.NewService,
			userinput.NewService,
			provideHooksService,
			provideProvidersService,
			fetchproviders.NewService,
			searchproviders.NewService,
			policy.NewService,
			mcp.NewConnectionService,
			oauthclients.NewRegistry,
			pluginspkg.NewService,
			mcp.NewToolSessionContextStore,
			conversation.NewService,
			event.NewHub,
			provideAudioRegistry,
			audiopkg.NewService,
			provideVideoRegistry,
			videopkg.NewService,
			provideAudioTempStore,
			provideSessionService,
			provideMessageService,
			provideMediaService,
			provideAgent,
			provideChatResolver,
			provideTurnService,
			provideScheduleTriggerer,
			provideHeartbeatSessionCreator,
			provideScheduleSessionCreator,
			schedule.NewService,
			provideHeartbeatTriggerer,
			heartbeat.NewService,
			compaction.NewService,
			provideContainerdHandler,
			provideBotBackupService,
			provideFederationGateway,
			provideACPToolSource,
			provideToolGatewayService,
			provideBackgroundManager,
			provideToolProviders,
			provideOAuthService,
		),
		fx.Invoke(
			injectToolProviders,
			injectACPToolProviders,
			startRegistrySync,
			startAudioProviderBootstrap,
			startVideoProviderBootstrap,
			startMemoryProviderBootstrap,
			startFetchProviderBootstrap,
			startSearchProviderBootstrap,
			startScheduleService,
			startHeartbeatService,
			startContainerReconciliation,
			startBackgroundTaskCleanup,
			startAudioTempStoreCleanup,
		),
	)
}
