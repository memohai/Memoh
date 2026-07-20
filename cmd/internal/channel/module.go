package channel

import (
	"go.uber.org/fx"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/inbound"
	emailpkg "github.com/memohai/memoh/internal/email"
	"github.com/memohai/memoh/internal/rpc/serverruntime"
	"github.com/memohai/memoh/internal/webhooktunnel"
)

// Module assembles the shared Channel boundary providers: registry,
// manager, lifecycle, inbound processing, discuss pipeline, email, and
// webhook tunnel. Turn execution is consumed through the injected
// turn.Service; this module never touches the resolver or agent directly.
func FoundationModule() fx.Option {
	return fx.Options(
		fx.Provide(
			identities.NewService,
			emailpkg.NewDBOAuthTokenStore,
			provideEmailRegistry,
			emailpkg.NewService,
			emailpkg.NewOutboxService,
			provideRouteService,
			providePipeline,
			provideEventStore,
			provideDiscussDriver,
			local.NewRouteHub,
			provideChannelRegistry,
			channel.NewStore,
		),
	)
}

// ServerLocalModule supplies the local Web channel path. It does not start any
// external channel or email connections.
func ServerLocalModule() fx.Option {
	return fx.Options(
		fx.Provide(
			provideCommandHandler,
			provideLocalCommandHandler,
			provideLocalSkillResolver,
			provideLocalChannelAudio,
			provideLocalChannelSettings,
			provideChannelRouter,
			provideChannelManager,
		),
	)
}

// RuntimeModule supplies the standalone Channel process. Agent-facing command,
// skill, audio, and turn work arrive through the Server RPC client.
func RuntimeModule() fx.Option {
	return fx.Options(
		fx.Provide(
			provideLocalMediaService,
			provideRemoteCommandHandler,
			provideRemoteSkillResolver,
			provideRemoteChannelAudio,
			provideStandaloneChannelSettings,
			provideEmailChatGateway,
			provideEmailTrigger,
			emailpkg.NewManager,
			provideChannelRouter,
			provideChannelManager,
			provideChannelLifecycleService,
			provideLocalChannelRuntime,
			provideChannelRuntimeInterface,
			provideEmailRuntimeInterface,
			webhooktunnel.NewManager,
		),
		fx.Invoke(
			startChannelManager,
			startEmailManager,
			startWebhookTunnelListener,
			startWebhookTunnel,
		),
	)
}

// Module preserves the previous all-in-one assembly for focused tests.
func Module() fx.Option {
	return fx.Options(FoundationModule(), ServerLocalModule())
}

func provideLocalChannelRuntime(lifecycle *channel.Lifecycle, store *channel.Store, manager *channel.Manager) *channel.LocalRuntime {
	return &channel.LocalRuntime{Lifecycle: lifecycle, Store: store, Manager: manager}
}

func provideChannelRuntimeInterface(runtime *channel.LocalRuntime) channel.Runtime { return runtime }

func provideEmailRuntimeInterface(manager *emailpkg.Manager) emailpkg.Runtime { return manager }

func provideRemoteCommandHandler(client *serverruntime.Client) inbound.CommandHandler { return client }

func provideRemoteSkillResolver(client *serverruntime.Client) inbound.RequestedSkillResolver {
	return client
}

func provideRemoteChannelAudio(client *serverruntime.Client) channelAudio { return client }
