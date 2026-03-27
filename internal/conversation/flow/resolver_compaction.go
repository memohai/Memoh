package flow

import (
	"context"
	"log/slog"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
)

func (r *Resolver) maybeCompact(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, inputTokens int) {
	if r.compactionService == nil || r.settingsService == nil {
		return
	}
	settings, err := r.settingsService.GetBot(ctx, req.BotID)
	if err != nil {
		r.logger.Warn("compaction: failed to load settings", slog.Any("error", err))
		return
	}
	if !settings.CompactionEnabled || settings.CompactionThreshold <= 0 {
		return
	}
	if !compaction.ShouldCompact(inputTokens, settings.CompactionThreshold) {
		return
	}

	modelID := settings.CompactionModelID
	if modelID == "" {
		modelID = rc.model.ID
	}

	cfg := compaction.TriggerConfig{
		BotID:     req.BotID,
		SessionID: req.SessionID,
	}

	model, err := r.modelsService.GetByID(ctx, modelID)
	if err != nil {
		r.logger.Warn("compaction: failed to resolve model", slog.Any("error", err))
		return
	}
	cfg.ModelID = model.ModelID

	provider, err := models.FetchProviderByID(ctx, r.queries, model.LlmProviderID)
	if err != nil {
		r.logger.Warn("compaction: failed to fetch provider", slog.Any("error", err))
		return
	}
	authResolver := providers.NewService(nil, r.queries, "")
	creds, err := authResolver.ResolveModelCredentials(ctx, provider)
	if err != nil {
		r.logger.Warn("compaction: failed to resolve provider credentials", slog.Any("error", err))
		return
	}
	cfg.ClientType = provider.ClientType
	cfg.APIKey = creds.APIKey
	cfg.CodexAccountID = creds.CodexAccountID
	cfg.BaseURL = provider.BaseUrl

	r.compactionService.TriggerCompaction(ctx, cfg)
}
