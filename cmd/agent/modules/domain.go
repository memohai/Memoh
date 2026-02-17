package modules

import (
	"log/slog"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/preauth"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/subagent"
	"go.uber.org/fx"
)


var DomainModule = fx.Module(
    "domain",
    fx.Provide(
        models.NewService,
        bots.NewService,
        accounts.NewService,
        settings.NewService,
        providers.NewService,
        searchproviders.NewService,
        policy.NewService,
        preauth.NewService,
        mcp.NewConnectionService,
        subagent.NewService,
        conversation.NewService,
        identities.NewService,
        bind.NewService,
        event.NewHub,

        provideRouteService,
        provideMessageService,
    ),
)

// ---------------------------------------------------------------------------
// domain service providers (interface adapters)
// ---------------------------------------------------------------------------

func provideRouteService(log *slog.Logger, queries *dbsqlc.Queries, chatService *conversation.Service) *route.DBService {
	return route.NewService(log, queries, chatService)
}

func provideMessageService(log *slog.Logger, queries *dbsqlc.Queries, hub *event.Hub) *message.DBService {
	return message.NewService(log, queries, hub)
}

