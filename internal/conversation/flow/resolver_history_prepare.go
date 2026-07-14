package flow

import (
	"context"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
)

type preparedHistoryContext struct {
	messages          []conversation.ModelMessage
	records           []historyfrag.HistoryRecord
	estimatedTokens   int
	compactableTokens int
}

func (r *Resolver) prepareHistoryContext(
	ctx context.Context,
	req conversation.ChatRequest,
	fallback historyfrag.ScopeFallback,
	contextTokenBudget int,
) (preparedHistoryContext, error) {
	loaded, err := r.loadHistoryRecords(ctx, fallback, req.SessionID, defaultMaxContextMinutes)
	if err != nil {
		return preparedHistoryContext{}, err
	}
	loaded = pruneHistoryForGateway(loaded)
	boundary := r.loadCompactionArtifactBoundary(ctx, loaded, req.SessionID, req.HistoryCutoffBeforeMessageID)
	loaded = filterMessagesBeforeID(loaded, req.HistoryCutoffBeforeMessageID)
	loaded = dedupePersistedCurrentUserMessage(loaded, req)
	loaded, err = r.ensureRequiredHistoryMessage(ctx, loaded, req)
	if err != nil {
		return preparedHistoryContext{}, err
	}
	loaded, err = r.replaceCompactedMessages(
		ctx,
		req.SessionID,
		compactionSummaryScope(req.BotID, req.ChatID, req.SessionID, req.ConversationType, req.ConversationName, req.ReplyTarget),
		loaded,
		boundary,
	)
	if err != nil {
		return preparedHistoryContext{}, err
	}
	messages, records, estimatedTokens := trimMessagesAndRecordsByTokens(r.logger, loaded, contextTokenBudget)
	return preparedHistoryContext{
		messages:          messages,
		records:           records,
		estimatedTokens:   estimatedTokens,
		compactableTokens: totalCompactableHistoryTokens(loaded),
	}, nil
}
