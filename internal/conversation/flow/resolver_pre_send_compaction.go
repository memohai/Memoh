package flow

import (
	"context"
	"log/slog"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
)

func (r *Resolver) buildRawHistoryContext(
	ctx context.Context,
	req conversation.ChatRequest,
	fallback historyfrag.ScopeFallback,
) (historyContextBuild, error) {
	loaded, err := r.loadHistoryRecords(ctx, fallback, req.SessionID, defaultMaxContextMinutes)
	if err != nil {
		return historyContextBuild{}, err
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded = filterMessagesBeforeID(loaded, req.HistoryCutoffBeforeMessageID)
	loaded = dedupePersistedCurrentUserMessage(loaded, req)
	loaded, err = r.ensureRequiredHistoryMessage(ctx, loaded, req)
	if err != nil {
		return historyContextBuild{}, err
	}
	loaded, err = r.replaceCompactedMessages(
		ctx,
		req.SessionID,
		compactionSummaryScope(
			req.BotID,
			req.ChatID,
			req.SessionID,
			req.ConversationType,
			req.ConversationName,
			req.ReplyTarget,
		),
		loaded,
	)
	if err != nil {
		return historyContextBuild{}, err
	}
	return assembleHistoryContext(r.logger, loaded, nil)
}

func (r *Resolver) drainRawHistoryContext(
	ctx context.Context,
	req conversation.ChatRequest,
	fallback historyfrag.ScopeFallback,
	initial historyContextBuild,
	contextTokenBudget int,
) (historyContextBuild, bool, error) {
	drained, _, attempted, err := drainPreSendCompaction(
		ctx,
		initial,
		initial.Allocation.CompactableTokens,
		preSendCompactionThreshold(contextTokenBudget),
		func(ctx context.Context, pressure int) (compaction.Result, error) {
			return r.runCompactionSyncResult(ctx, req, pressure, contextTokenBudget)
		},
		func(ctx context.Context) (historyContextBuild, int, error) {
			rebuilt, err := r.buildRawHistoryContext(ctx, req, fallback)
			return rebuilt, rebuilt.Allocation.CompactableTokens, err
		},
	)
	return drained, attempted, err
}

func (r *Resolver) drainPipelineHistoryContext(
	ctx context.Context,
	req conversation.ChatRequest,
	initial pipelineContextBuild,
	contextTokenBudget int,
) (pipelineContextBuild, bool, error) {
	drained, _, attempted, err := drainPreSendCompaction(
		ctx,
		initial,
		initial.Allocation.CompactableTokens,
		preSendCompactionThreshold(contextTokenBudget),
		func(ctx context.Context, pressure int) (compaction.Result, error) {
			return r.runCompactionSyncResult(ctx, req, pressure, contextTokenBudget)
		},
		func(ctx context.Context) (pipelineContextBuild, int, error) {
			rebuilt, err := r.buildPipelineContext(ctx, req, 0)
			return rebuilt, rebuilt.Allocation.CompactableTokens, err
		},
	)
	return drained, attempted, err
}

func preSendCompactionThreshold(contextTokenBudget int) int {
	if contextTokenBudget <= 0 {
		return 0
	}
	return contextTokenBudget * compactionBudgetThresholdPercent / 100
}

func (r *Resolver) runCompactionSyncResult(
	ctx context.Context,
	req conversation.ChatRequest,
	inputTokens int,
	contextTokenBudget int,
) (compaction.Result, error) {
	if r.syncCompactionFn != nil {
		return r.syncCompactionFn(ctx, req, inputTokens, contextTokenBudget)
	}
	if r.compactionService == nil || r.settingsService == nil {
		r.logger.Warn("compaction sync: skipped, service or settings nil")
		return compaction.Result{Status: compaction.StatusNoop}, nil
	}
	botSettings, err := r.settingsService.GetBot(ctx, req.BotID)
	if err != nil {
		r.logger.Warn("compaction sync: failed to load settings", slog.Any("error", err))
		return compaction.Result{}, err
	}
	if !botSettings.CompactionEnabled {
		r.logger.Warn("compaction sync: compaction disabled, skipping")
		return compaction.Result{Status: compaction.StatusNoop}, nil
	}
	cfg, err := r.buildCompactionConfig(ctx, req, botSettings, inputTokens)
	if err != nil {
		r.logger.Warn("compaction sync: failed to build config", slog.Any("error", err))
		return compaction.Result{}, err
	}
	if cfg.ModelID == "" {
		return compaction.Result{Status: compaction.StatusNoop}, nil
	}
	cfg.TargetTokens = syncCompactionTargetTokens(contextTokenBudget, cfg.Ratio)
	r.logger.Info("compaction sync: running synchronously",
		slog.String("bot_id", req.BotID),
		slog.String("session_id", req.SessionID),
		slog.Int("input_tokens", inputTokens),
		slog.String("model_id", cfg.ModelID),
	)
	result, err := r.compactionService.RunCompactionSync(ctx, cfg)
	if err != nil {
		r.logger.Warn("compaction sync: failed", slog.Any("error", err))
		return result, err
	}
	r.logger.Info("compaction sync: completed",
		slog.String("bot_id", req.BotID),
		slog.String("session_id", req.SessionID),
		slog.String("status", result.Status),
	)
	return result, nil
}
