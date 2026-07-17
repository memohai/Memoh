package appchannel

import (
	"go.uber.org/fx"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/identities"
	emailpkg "github.com/memohai/memoh/internal/email"
	"github.com/memohai/memoh/internal/webhooktunnel"
)

// Module is the Channel boundary composition root (spec §7.1): channel
// registry/manager/lifecycle, the inbound processor, the discuss
// pipeline, email, and the webhook tunnel. Turn execution is consumed
// through the injected turn.Service; this module never touches the
// resolver or agent directly.
func Module() fx.Option {
	return fx.Options(
		fx.Provide(
			identities.NewService,
			emailpkg.NewDBOAuthTokenStore,
			provideEmailRegistry,
			emailpkg.NewService,
			emailpkg.NewOutboxService,
			provideEmailChatGateway,
			provideEmailTrigger,
			emailpkg.NewManager,
			provideRouteService,
			providePipeline,
			provideEventStore,
			provideDiscussDriver,
			local.NewRouteHub,
			provideChannelRegistry,
			channel.NewStore,
			provideCommandHandler,
			provideChannelRouter,
			provideChannelManager,
			provideChannelLifecycleService,
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
