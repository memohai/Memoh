package flow

import (
	"context"
	"log/slog"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/settings"
)

func (r *Resolver) maybeCompact(ctx context.Context, req conversation.ChatRequest, _ resolvedContext, inputTokens int) {
	if r.compactionService == nil || r.settingsService == nil {
		r.logger.Info("compaction: skipped, service or settings nil")
		return
	}
	botSettings, err := r.settingsService.GetBot(ctx, req.BotID)
	if err != nil {
		r.logger.Warn("compaction: failed to load settings", slog.Any("error", err))
		return
	}
	if !botSettings.CompactionEnabled || botSettings.CompactionThreshold <= 0 {
		r.logger.Info("compaction: skipped, disabled or no threshold",
			slog.Bool("enabled", botSettings.CompactionEnabled),
			slog.Int("threshold", botSettings.CompactionThreshold),
		)
		return
	}
	if !compaction.ShouldCompact(inputTokens, botSettings.CompactionThreshold) {
		r.logger.Info("compaction: skipped, below threshold",
			slog.Int("input_tokens", inputTokens),
			slog.Int("threshold", botSettings.CompactionThreshold),
		)
		return
	}

	r.logger.Info("compaction: triggering",
		slog.String("bot_id", req.BotID),
		slog.String("session_id", req.SessionID),
		slog.Int("input_tokens", inputTokens),
		slog.Int("threshold", botSettings.CompactionThreshold),
		slog.Int("ratio", botSettings.CompactionRatio),
	)

	cfg, err := r.buildCompactionConfig(ctx, req, botSettings, inputTokens)
	if err != nil {
		r.logger.Warn("compaction: failed to build config", slog.Any("error", err))
		return
	}
	r.compactionService.TriggerCompaction(ctx, cfg)
}

// runCompactionSync runs compaction synchronously when context reaches
// 70% of the model's context window. It blocks until compaction completes.
func (r *Resolver) runCompactionSync(ctx context.Context, req conversation.ChatRequest, inputTokens int) {
	if r.compactionService == nil || r.settingsService == nil {
		r.logger.Warn("compaction sync: skipped, service or settings nil")
		return
	}
	botSettings, err := r.settingsService.GetBot(ctx, req.BotID)
	if err != nil {
		r.logger.Warn("compaction sync: failed to load settings", slog.Any("error", err))
		return
	}
	if !botSettings.CompactionEnabled {
		r.logger.Warn("compaction sync: compaction disabled, skipping")
		return
	}

	cfg, err := r.buildCompactionConfig(ctx, req, botSettings, inputTokens)
	if err != nil {
		r.logger.Warn("compaction sync: failed to build config", slog.Any("error", err))
		return
	}

	r.logger.Info("compaction sync: running synchronously",
		slog.String("bot_id", req.BotID),
		slog.String("session_id", req.SessionID),
		slog.Int("input_tokens", inputTokens),
		slog.String("model_id", cfg.ModelID),
	)

	if err := r.compactionService.RunCompactionSync(ctx, cfg); err != nil {
		r.logger.Warn("compaction sync: failed", slog.Any("error", err))
	} else {
		r.logger.Info("compaction sync: completed successfully",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
		)
	}
}

// buildCompactionConfig resolves the compaction model, provider credentials,
// and sets MaxCompactTokens to 90% of the compaction model's context window.
func (r *Resolver) buildCompactionConfig(ctx context.Context, req conversation.ChatRequest, botSettings settings.Settings, inputTokens int) (compaction.TriggerConfig, error) {
	modelID := botSettings.CompactionModelID
	if modelID == "" {
		return compaction.TriggerConfig{}, nil
	}

	ratio := botSettings.CompactionRatio
	if ratio <= 0 || ratio > 100 {
		ratio = 80
	}

	compactModel, err := r.modelsService.GetByID(ctx, modelID)
	if err != nil {
		return compaction.TriggerConfig{}, err
	}

	compactProvider, err := models.FetchProviderByID(ctx, r.queries, compactModel.ProviderID)
	if err != nil {
		return compaction.TriggerConfig{}, err
	}
	authResolver := providers.NewService(nil, r.queries, "")
	authCtx := oauthctx.WithUserID(ctx, req.UserID)
	creds, err := authResolver.ResolveModelCredentials(authCtx, compactProvider)
	if err != nil {
		return compaction.TriggerConfig{}, err
	}

	cfg := compaction.TriggerConfig{
		BotID:            req.BotID,
		SessionID:        req.SessionID,
		ModelID:          compactModel.ModelID,
		ClientType:       compactProvider.ClientType,
		APIKey:           creds.APIKey,
		CodexAccountID:   creds.CodexAccountID,
		BaseURL:          providers.ProviderConfigString(compactProvider, "base_url"),
		Ratio:            ratio,
		TotalInputTokens: inputTokens,
		HTTPClient:       r.streamHTTPClient,
	}

	// Cap compaction input to 90% of the compaction model's context window.
	if compactModel.Config.ContextWindow != nil && *compactModel.Config.ContextWindow > 0 {
		cfg.MaxCompactTokens = *compactModel.Config.ContextWindow * 90 / 100
	}
	// For sync compaction: keep only the last few messages (~2000 tokens ≈ 3 messages).
	// The summary provides reference context; if the LLM needs details,
	// it will use tools (memory_read, search) to look them up.
	cfg.TargetTokens = 2000

	return cfg, nil
}
