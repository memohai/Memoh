package modules

import (
	"context"
	"log/slog"
	"time"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/memory"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/settings"
	"go.uber.org/fx"
)

var ConversationModule = fx.Module(
	"conversation",
	fx.Provide(
		provideChatResolver,
		provideScheduleTriggerer,
		schedule.NewService,
	),
	fx.Invoke(startScheduleService),
)

// ---------------------------------------------------------------------------
// conversation flow
// ---------------------------------------------------------------------------

func provideChatResolver(log *slog.Logger, cfg config.Config, modelsService *models.Service, queries *dbsqlc.Queries, memoryService *memory.Service, chatService *conversation.Service, msgService *message.DBService, settingsService *settings.Service, containerdHandler *handlers.ContainerdHandler) *flow.Resolver {
	resolver := flow.NewResolver(log, modelsService, queries, memoryService, chatService, msgService, settingsService, cfg.AgentGateway.BaseURL(), 120*time.Second)
	resolver.SetSkillLoader(&skillLoaderAdapter{handler: containerdHandler})
	return resolver
}

func provideScheduleTriggerer(resolver *flow.Resolver) schedule.Triggerer {
	return flow.NewScheduleGateway(resolver)
}

func startScheduleService(lc fx.Lifecycle, scheduleService *schedule.Service) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return scheduleService.Bootstrap(ctx)
		},
	})
}

// skillLoaderAdapter bridges handlers.ContainerdHandler to flow.SkillLoader.
type skillLoaderAdapter struct {
	handler *handlers.ContainerdHandler
}

func (a *skillLoaderAdapter) LoadSkills(ctx context.Context, botID string) ([]flow.SkillEntry, error) {
	items, err := a.handler.LoadSkills(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries := make([]flow.SkillEntry, len(items))
	for i, item := range items {
		entries[i] = flow.SkillEntry{
			Name:        item.Name,
			Description: item.Description,
			Content:     item.Content,
			Metadata:    item.Metadata,
		}
	}
	return entries, nil
}
