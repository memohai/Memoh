package flow

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

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

func trimMessagesByTokens(log *slog.Logger, messages []historyfrag.HistoryRecord, maxTokens int) ([]conversation.ModelMessage, int) {
	trimmed, _, totalTokens := trimMessagesAndRecordsByTokens(log, messages, maxTokens)
	return trimmed, totalTokens
}

// totalCompactableHistoryTokens estimates the tokens held by raw history rows
// only. Active summaries are excluded: compaction can never shrink them, so
// counting them toward the compaction trigger would re-fire it on every
// request once accumulated summaries alone cross the threshold.
func totalCompactableHistoryTokens(records []historyfrag.HistoryRecord) int {
	total := 0
	for _, record := range records {
		if record.Kind == contextfrag.KindConversationSummary || record.Lifecycle == historyfrag.LifecycleActiveSummary {
			continue
		}
		total += estimateMessageTokens(record.ModelMessage)
	}
	return total
}

func trimMessagesAndRecordsByTokens(log *slog.Logger, messages []historyfrag.HistoryRecord, maxTokens int) ([]conversation.ModelMessage, []historyfrag.HistoryRecord, int) {
	if maxTokens == 0 || len(messages) == 0 {
		totalTokens := 0
		for _, m := range messages {
			totalTokens += estimateMessageTokens(m.ModelMessage)
		}
		return historyfrag.ToModelMessages(messages), messages, totalTokens
	}

	// Scan from newest to oldest, accumulating per-message estimated context
	// token costs. Each message's cost represents the tokens it occupies in the
	// context window (not the output tokens it generated). We use a character-
	// based estimate for all messages since this measures context window impact.
	scannedTokens := 0
	cutoff := 0
	for i := len(messages) - 1; i >= 0; i-- {
		scannedTokens += estimateMessageTokens(messages[i].ModelMessage)
		if scannedTokens > maxTokens {
			cutoff = i + 1
			break
		}
	}

	// Keep provider-valid message order: a "tool" message must follow a preceding
	// assistant tool call. When history is head-trimmed, a leading tool message
	// may become orphaned and cause provider 400 errors.
	for cutoff < len(messages) && strings.EqualFold(strings.TrimSpace(messages[cutoff].ModelMessage.Role), "tool") {
		cutoff++
	}
	cutoff, _ = fitRequiredMessagesWithinBudget(messages, cutoff, maxTokens)

	forceKeptPrefix := forceKeptMessagesBeforeCutoff(messages, cutoff)
	retained := make([]historyfrag.HistoryRecord, 0, len(messages)-cutoff+len(forceKeptPrefix))
	retained = append(retained, forceKeptPrefix...)
	retained = append(retained, messages[cutoff:]...)
	result := make([]conversation.ModelMessage, 0, len(retained))
	if cutoff > 0 {
		notice := historyTruncationNotice()
		result = append(result, notice)
	}
	for _, m := range retained {
		result = append(result, m.ModelMessage)
	}
	totalTokens := 0
	for _, message := range result {
		totalTokens += estimateMessageTokens(message)
	}
	if cutoff > 0 && log != nil {
		log.Info("trimMessagesByTokens: context trimmed",
			slog.Int("total_messages", len(messages)),
			slog.Int("estimated_tokens", totalTokens),
			slog.Int("max_tokens", maxTokens),
			slog.Int("cutoff_index", cutoff),
			slog.Int("kept_messages", len(retained)),
		)
	}
	return result, retained, totalTokens
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

func fitRequiredMessagesWithinBudget(messages []historyfrag.HistoryRecord, cutoff int, maxTokens int) (int, int) {
	if maxTokens <= 0 || len(messages) == 0 {
		return cutoff, estimateMessagesTokens(messages)
	}
	if cutoff < 0 {
		cutoff = 0
	}
	if cutoff > len(messages) {
		cutoff = len(messages)
	}
	for {
		requiredPrefix := requiredMessagesBeforeCutoff(messages, cutoff)
		totalTokens := estimateMessagesTokens(requiredPrefix) + estimateMessagesTokens(messages[cutoff:])
		if totalTokens <= maxTokens || cutoff >= len(messages) {
			return cutoff, totalTokens
		}
		cutoff++
		for cutoff < len(messages) && strings.EqualFold(strings.TrimSpace(messages[cutoff].ModelMessage.Role), "tool") {
			cutoff++
		}
	}
}

func requiredMessagesBeforeCutoff(messages []historyfrag.HistoryRecord, cutoff int) []historyfrag.HistoryRecord {
	if cutoff <= 0 {
		return nil
	}
	if cutoff > len(messages) {
		cutoff = len(messages)
	}
	var required []historyfrag.HistoryRecord
	for _, message := range messages[:cutoff] {
		if message.Required {
			required = append(required, message)
		}
	}
	return required
}

func estimateMessagesTokens(messages []historyfrag.HistoryRecord) int {
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m.ModelMessage)
	}
	return total
}

func forceKeptMessagesBeforeCutoff(messages []historyfrag.HistoryRecord, cutoff int) []historyfrag.HistoryRecord {
	if cutoff <= 0 {
		return nil
	}
	if cutoff > len(messages) {
		cutoff = len(messages)
	}
	var forceKept []historyfrag.HistoryRecord
	for _, m := range messages[:cutoff] {
		if m.Required || m.Kind == contextfrag.KindConversationSummary || m.Lifecycle == historyfrag.LifecycleActiveSummary {
			forceKept = append(forceKept, m)
		}
	}
	return forceKept
}
