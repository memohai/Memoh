package flow

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messageconv"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

func (r *Resolver) loadHistoryRecords(ctx context.Context, fallback historyfrag.ScopeFallback, sessionID string, maxContextMinutes int) ([]historyfrag.HistoryRecord, error) {
	if r.messageService == nil {
		return nil, nil
	}
	since := time.Now().UTC().Add(-time.Duration(maxContextMinutes) * time.Minute)
	var msgs []messagepkg.Message
	var err error
	if strings.TrimSpace(sessionID) != "" {
		msgs, err = r.messageService.ListActiveSinceBySession(ctx, sessionID, since)
	} else {
		msgs, err = r.messageService.ListActiveSince(ctx, fallback.ChatID, since)
	}
	if err != nil {
		return nil, err
	}
	result := make([]historyfrag.HistoryRecord, 0, len(msgs))
	for _, m := range msgs {
		record, err := historyfrag.FromDBMessageWithLogger(r.logger, m, fallback)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func historyScopeFallbackFromChatRequest(req conversation.ChatRequest) historyfrag.ScopeFallback {
	return historyfrag.ScopeFallback{
		ChatID:           strings.TrimSpace(req.ChatID),
		ConversationType: strings.TrimSpace(req.ConversationType),
		ConversationName: strings.TrimSpace(req.ConversationName),
		ReplyTarget:      strings.TrimSpace(req.ReplyTarget),
	}
}

func historyScopeFallbackFromUserInputRequest(req userinput.Request) historyfrag.ScopeFallback {
	return historyfrag.ScopeFallback{
		ConversationType: strings.TrimSpace(req.ConversationType),
		ReplyTarget:      strings.TrimSpace(req.ReplyTarget),
	}
}

func historyScopeFallbackFromToolApprovalRequest(req toolapproval.Request) historyfrag.ScopeFallback {
	return historyfrag.ScopeFallback{
		ConversationType: strings.TrimSpace(req.ConversationType),
		ReplyTarget:      strings.TrimSpace(req.ReplyTarget),
	}
}

func (r *Resolver) ensureRequiredHistoryMessage(ctx context.Context, messages []historyfrag.HistoryRecord, req conversation.ChatRequest) ([]historyfrag.HistoryRecord, error) {
	messageID := strings.TrimSpace(req.RequiredHistoryMessageID)
	if messageID == "" || r.messageService == nil || strings.TrimSpace(req.SessionID) == "" {
		return messages, nil
	}
	for i, item := range messages {
		if strings.TrimSpace(item.DBMessageID) == messageID {
			messages[i].Required = true
			return messages, nil
		}
	}
	window, err := r.messageService.ListVisibleFromBySession(ctx, req.SessionID, messageID)
	if err != nil {
		return nil, err
	}
	fallback := historyScopeFallbackFromChatRequest(req)
	required := make([]historyfrag.HistoryRecord, 0, len(window))
	for _, msg := range window {
		record, err := historyfrag.FromDBMessage(msg, fallback)
		if err != nil {
			return nil, err
		}
		required = append(required, record)
	}
	required = pruneHistoryForGateway(required)
	required = filterMessagesBeforeID(required, req.HistoryCutoffBeforeMessageID)
	if !containsHistoryRecord(required, messageID) {
		return nil, errors.New("required history message is not visible")
	}
	for i := range required {
		if strings.TrimSpace(required[i].DBMessageID) == messageID {
			required[i].Required = true
		}
	}
	return mergeRequiredHistoryWindow(messages, required), nil
}

func containsHistoryRecord(messages []historyfrag.HistoryRecord, messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	for _, item := range messages {
		if strings.TrimSpace(item.DBMessageID) == messageID {
			return true
		}
	}
	return false
}

func mergeRequiredHistoryWindow(messages []historyfrag.HistoryRecord, required []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	if len(required) == 0 {
		return messages
	}
	requiredIDs := make(map[string]struct{}, len(required))
	for _, item := range required {
		if id := strings.TrimSpace(item.DBMessageID); id != "" {
			requiredIDs[id] = struct{}{}
		}
	}
	merged := make([]historyfrag.HistoryRecord, 0, len(messages)+len(required))
	for _, item := range messages {
		if _, ok := requiredIDs[strings.TrimSpace(item.DBMessageID)]; ok {
			continue
		}
		merged = append(merged, item)
	}
	return append(merged, required...)
}

func filterMessagesBeforeID(messages []historyfrag.HistoryRecord, messageID string) []historyfrag.HistoryRecord {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return messages
	}
	for i, item := range messages {
		if strings.TrimSpace(item.DBMessageID) == messageID {
			return messages[:i]
		}
	}
	return messages
}

func compactionSummaryScope(botID, chatID, sessionID, conversationType, conversationName, replyTarget string) contextfrag.Scope {
	return contextfrag.Scope{
		BotID:            strings.TrimSpace(botID),
		ChatID:           strings.TrimSpace(chatID),
		SessionID:        strings.TrimSpace(sessionID),
		ConversationType: strings.TrimSpace(conversationType),
		ConversationName: strings.TrimSpace(conversationName),
		ReplyTarget:      strings.TrimSpace(replyTarget),
	}
}

func dedupePersistedCurrentUserMessage(messages []historyfrag.HistoryRecord, req conversation.ChatRequest) []historyfrag.HistoryRecord {
	if !req.UserMessagePersisted || len(messages) == 0 {
		return messages
	}

	targetSessionID := strings.TrimSpace(req.SessionID)
	targetExternalID := strings.TrimSpace(req.ExternalMessageID)
	targetPlatform := strings.TrimSpace(req.CurrentChannel)
	targetSenderChannelID := strings.TrimSpace(req.SourceChannelIdentityID)
	if targetExternalID == "" {
		return messages
	}

	for i := len(messages) - 1; i >= 0; i-- {
		item := messages[i]
		if !strings.EqualFold(strings.TrimSpace(item.ModelMessage.Role), "user") {
			continue
		}
		if strings.TrimSpace(item.ExternalMessageID) != targetExternalID {
			continue
		}
		if targetSessionID != "" && item.SessionID != "" && item.SessionID != targetSessionID {
			continue
		}
		if targetPlatform != "" && item.Platform != "" && !strings.EqualFold(item.Platform, targetPlatform) {
			continue
		}
		if targetSenderChannelID != "" && item.SenderChannelIdentityID != "" && item.SenderChannelIdentityID != targetSenderChannelID {
			continue
		}
		return append(messages[:i], messages[i+1:]...)
	}

	return messages
}

func estimateMessageTokens(msg conversation.ModelMessage) int {
	return messageconv.EstimateModelMessageTokens(msg)
}

type historyContextBuild struct {
	Messages       []conversation.ModelMessage
	HistoryRecords []historyfrag.HistoryRecord
	Allocation     contextbudget.Allocation
	EmittedTokens  int
	Projection     budgetSourceProjection
}

func assembleHistoryContext(log *slog.Logger, records []historyfrag.HistoryRecord, envelopeLimit *int) (historyContextBuild, error) {
	projection := budgetSourcesForHistoryRecords(records)
	assembled, err := assembleBudgetSources(
		projection,
		envelopeLimit,
		historyTruncationNotice().TextContent(),
	)
	build := historyContextBuild{
		Messages:       assembled.messages,
		Allocation:     assembled.allocation,
		EmittedTokens:  assembled.emittedTokens,
		HistoryRecords: retainedHistoryRecords(records, assembled.sourceIndexes),
		Projection:     projection,
	}
	if log != nil && build.Allocation.BudgetTrimmed {
		limit := 0
		if envelopeLimit != nil {
			limit = max(*envelopeLimit, 0)
		}
		log.Info("assemble history context: sources trimmed",
			slog.Int("total_sources", len(records)),
			slog.Int("kept_sources", len(build.HistoryRecords)),
			slog.Int("dropped_sources", len(build.Allocation.Dropped)),
			slog.Int("emitted_tokens", build.EmittedTokens),
			slog.Int("envelope_limit", limit),
		)
	}
	return build, err
}

func historyTruncationNotice() conversation.ModelMessage {
	return conversation.ModelMessage{
		Role: "system",
		Content: conversation.NewTextContent(
			"[System Notice] Earlier conversation history has been trimmed to fit the context window. " +
				"If you need information from earlier in the conversation, use the available tools " +
				"(such as memory_read or web search) to retrieve it.",
		),
	}
}

func retainedHistoryRecords(records []historyfrag.HistoryRecord, sourceIndexes []int) []historyfrag.HistoryRecord {
	retained := make([]historyfrag.HistoryRecord, 0, len(sourceIndexes))
	seen := make([]bool, len(records))
	for _, sourceIndex := range sourceIndexes {
		if sourceIndex < 0 || sourceIndex >= len(records) || seen[sourceIndex] {
			continue
		}
		seen[sourceIndex] = true
		retained = append(retained, records[sourceIndex])
	}
	return retained
}

func trimMessagesByTokens(log *slog.Logger, records []historyfrag.HistoryRecord, maxTokens int) ([]conversation.ModelMessage, int) {
	trimmed, _, totalTokens := trimMessagesAndRecordsByTokens(log, records, maxTokens)
	return trimmed, totalTokens
}

func trimMessagesAndRecordsByTokens(log *slog.Logger, records []historyfrag.HistoryRecord, maxTokens int) ([]conversation.ModelMessage, []historyfrag.HistoryRecord, int) {
	var envelopeLimit *int
	if maxTokens != 0 {
		envelopeLimit = &maxTokens
	}
	build, _ := assembleHistoryContext(log, records, envelopeLimit)
	return build.Messages, build.HistoryRecords, build.EmittedTokens
}
