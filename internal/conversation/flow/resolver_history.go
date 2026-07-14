package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
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
	text := msg.TextContent()
	if len(text) == 0 {
		data, _ := json.Marshal(msg.Content)
		return len(data) / 4
	}
	return len(text) / 4
}

func trimMessagesByTokens(log *slog.Logger, messages []historyfrag.HistoryRecord, maxTokens int) ([]conversation.ModelMessage, int) {
	if maxTokens == 0 || len(messages) == 0 {
		totalTokens := 0
		for _, m := range messages {
			totalTokens += estimateMessageTokens(m.ModelMessage)
		}
		return historyfrag.ToModelMessages(messages), totalTokens
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
	cutoff, totalTokens := fitRequiredMessagesWithinBudget(messages, cutoff, maxTokens)

	if cutoff > 0 && log != nil {
		log.Info("trimMessagesByTokens: context trimmed",
			slog.Int("total_messages", len(messages)),
			slog.Int("estimated_tokens", totalTokens),
			slog.Int("max_tokens", maxTokens),
			slog.Int("cutoff_index", cutoff),
			slog.Int("kept_messages", len(messages)-cutoff),
		)
	}

	requiredPrefix := requiredMessagesBeforeCutoff(messages, cutoff)
	result := make([]conversation.ModelMessage, 0, len(messages)-cutoff+len(requiredPrefix))
	if cutoff > 0 {
		// Add a truncation notice at the beginning so the LLM knows earlier
		// context was trimmed and it can use tools (memory, search) to look up
		// past information if needed.
		result = append(result, conversation.ModelMessage{
			Role: "system",
			Content: conversation.NewTextContent(
				"[System Notice] Earlier conversation history has been trimmed to fit the context window. " +
					"If you need information from earlier in the conversation, use the available tools " +
					"(such as memory_read or web search) to retrieve it.",
			),
		})
	}
	for _, m := range requiredPrefix {
		result = append(result, m.ModelMessage)
	}
	for _, m := range messages[cutoff:] {
		result = append(result, m.ModelMessage)
	}
	return result, totalTokens
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

func estimateMessagesTokens(messages []historyfrag.HistoryRecord) int {
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m.ModelMessage)
	}
	return total
}

func requiredMessagesBeforeCutoff(messages []historyfrag.HistoryRecord, cutoff int) []historyfrag.HistoryRecord {
	if cutoff <= 0 {
		return nil
	}
	if cutoff > len(messages) {
		cutoff = len(messages)
	}
	var required []historyfrag.HistoryRecord
	for _, m := range messages[:cutoff] {
		if m.Required {
			required = append(required, m)
		}
	}
	return required
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

func (r *Resolver) replaceCompactedMessages(ctx context.Context, scope contextfrag.Scope, messages []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	if r.queries == nil {
		return messages
	}

	compactGroups := make(map[string][]int) // compact_id -> indices
	for i, m := range messages {
		if m.CompactID != "" {
			compactGroups[m.CompactID] = append(compactGroups[m.CompactID], i)
		}
	}
	if len(compactGroups) == 0 {
		return messages
	}

	summaries := make(map[string]string)
	for compactID := range compactGroups {
		cUUID, err := db.ParseUUID(compactID)
		if err != nil {
			continue
		}
		log, err := r.queries.GetCompactionLogByID(ctx, cUUID)
		if err != nil {
			r.logger.Warn("replaceCompactedMessages: failed to load compact log", slog.String("compact_id", compactID), slog.Any("error", err))
			continue
		}
		if log.Status == "ok" && strings.TrimSpace(log.Summary) != "" {
			summaries[compactID] = log.Summary
		}
	}

	return replaceCompactedHistoryRecords(messages, summaries, scope)
}

func replaceCompactedHistoryRecords(messages []historyfrag.HistoryRecord, summaries map[string]string, scope contextfrag.Scope) []historyfrag.HistoryRecord {
	compactGroups := make(map[string][]int)
	requiredCompactGroups := make(map[string]bool)
	for i, m := range messages {
		if m.CompactID != "" {
			compactGroups[m.CompactID] = append(compactGroups[m.CompactID], i)
			if m.Required {
				requiredCompactGroups[m.CompactID] = true
			}
		}
	}
	if len(compactGroups) == 0 {
		return messages
	}

	var result []historyfrag.HistoryRecord
	replaced := make(map[string]bool)
	for _, m := range messages {
		if m.CompactID == "" {
			result = append(result, m)
			continue
		}
		if replaced[m.CompactID] {
			continue
		}
		replaced[m.CompactID] = true
		if requiredCompactGroups[m.CompactID] {
			for _, idx := range compactGroups[m.CompactID] {
				result = append(result, messages[idx])
			}
			continue
		}
		summary, ok := summaries[m.CompactID]
		if !ok || strings.TrimSpace(summary) == "" {
			for _, idx := range compactGroups[m.CompactID] {
				result = append(result, messages[idx])
			}
			continue
		}
		result = append(result, historyfrag.LegacySummaryRecord(m.CompactID, summary, scope))
	}
	return result
}

// buildMessagesFromPipeline assembles chat context from the DCP pipeline's
// RenderedContext (RC) merged with assistant/tool turns (TR) from
// bot_history_messages. This gives chat mode the same event-driven context
// that discuss mode uses, replacing the legacy loadMessages path.
func (r *Resolver) buildMessagesFromPipeline(ctx context.Context, req conversation.ChatRequest, contextTokenBudget int) []conversation.ModelMessage {
	sessionID := strings.TrimSpace(req.SessionID)
	if r.pipeline == nil || sessionID == "" {
		return nil
	}
	rc := r.pipeline.GetRC(sessionID)
	if len(rc) == 0 {
		return nil
	}

	trs := r.loadTurnResponses(ctx, sessionID)

	composed := pipelinepkg.ComposeContext(rc, trs, "")
	if composed == nil {
		return nil
	}

	messages := make([]conversation.ModelMessage, 0, len(composed.Messages))
	for _, m := range composed.Messages {
		contentJSON := m.RawContent
		if len(contentJSON) == 0 {
			var err error
			contentJSON, err = json.Marshal(m.Content)
			if err != nil {
				continue
			}
		}
		messages = append(messages, conversation.ModelMessage{
			Role:    m.Role,
			Content: contentJSON,
		})
	}

	// Apply context token budget trimming to pipeline path as well.
	if contextTokenBudget > 0 && len(messages) > 0 {
		messages = trimPipelineMessagesByTokens(r.logger, messages, contextTokenBudget)
	}

	return messages
}

// trimPipelineMessagesByTokens trims pipeline-assembled messages to fit within
// the context token budget using character-based estimation.
func trimPipelineMessagesByTokens(log *slog.Logger, messages []conversation.ModelMessage, maxTokens int) []conversation.ModelMessage {
	totalTokens := 0
	cutoff := 0
	for i := len(messages) - 1; i >= 0; i-- {
		totalTokens += estimateMessageTokens(messages[i])
		if totalTokens > maxTokens {
			cutoff = i + 1
			break
		}
	}

	// Avoid orphaned tool messages at the cutoff boundary.
	for cutoff < len(messages) && strings.EqualFold(strings.TrimSpace(messages[cutoff].Role), "tool") {
		cutoff++
	}

	if cutoff > 0 && log != nil {
		log.Info("trimPipelineMessagesByTokens: context trimmed",
			slog.Int("total_messages", len(messages)),
			slog.Int("estimated_tokens", totalTokens),
			slog.Int("max_tokens", maxTokens),
			slog.Int("kept_messages", len(messages)-cutoff),
		)
	}

	return messages[cutoff:]
}

// loadTurnResponses loads recent assistant/tool messages from bot_history_messages
// for use as the TR stream in pipeline-based context assembly.
func (r *Resolver) loadTurnResponses(ctx context.Context, sessionID string) []pipelinepkg.TurnResponseEntry {
	if r.messageService == nil {
		return nil
	}
	since := time.Now().UTC().Add(-24 * time.Hour)
	msgs, err := r.messageService.ListActiveSinceBySession(ctx, sessionID, since)
	if err != nil {
		r.logger.Warn("load TRs failed", slog.String("session_id", sessionID), slog.Any("error", err))
		return nil
	}
	var trs []pipelinepkg.TurnResponseEntry
	for _, m := range msgs {
		entry, ok := pipelinepkg.DecodeTurnResponseEntry(m)
		if !ok {
			continue
		}
		trs = append(trs, entry)
	}
	return trs
}

// stripToolMessages removes bulky tool interactions from the context while
// keeping ask_user calls and results. ask_user is conversation-visible: the
// question and the user's answer are part of the chat semantics, not tool noise.
func stripToolMessages(messages []conversation.ModelMessage) []conversation.ModelMessage {
	filtered := make([]conversation.ModelMessage, 0, len(messages))
	for _, m := range messages {
		role := strings.TrimSpace(m.Role)
		if strings.EqualFold(role, "tool") {
			if kept := keepAskUserToolResultMessage(m); kept != nil {
				filtered = append(filtered, *kept)
			}
			continue
		}
		// Remove assistant messages that only contain tool calls / reasoning with
		// no visible text. Tool-call metadata may live either in ToolCalls or in
		// structured content parts.
		if strings.EqualFold(role, "assistant") && hasToolCallContent(m) {
			stripped, ok := stripNonAskUserToolCalls(m)
			if !ok {
				continue
			}
			m = stripped
		}
		filtered = append(filtered, m)
	}
	return filtered
}

func stripNonAskUserToolCalls(message conversation.ModelMessage) (conversation.ModelMessage, bool) {
	legacyToolCalls := keepAskUserLegacyToolCalls(message.ToolCalls)
	text := strings.TrimSpace(message.TextContent())

	keptParts := filterAssistantContextParts(modelMessageToSDKMessage(message).Content)
	if len(keptParts) > 0 {
		message = modelMessageFromSDKParts(sdk.MessageRoleAssistant, keptParts, message.Usage)
		message.ToolCalls = legacyToolCalls
		return message, true
	}

	message.ToolCalls = legacyToolCalls
	if len(message.ToolCalls) > 0 {
		if text != "" {
			message.Content = conversation.NewTextContent(text)
		}
		return message, true
	}
	if text == "" {
		return conversation.ModelMessage{}, false
	}
	message.Content = conversation.NewTextContent(text)
	return message, true
}

func keepAskUserToolResultMessage(message conversation.ModelMessage) *conversation.ModelMessage {
	if strings.EqualFold(strings.TrimSpace(message.Name), userinput.ToolNameAskUser) {
		return &message
	}
	results := filterAskUserToolResults(modelMessageToSDKMessage(message).Content)
	if len(results) == 0 {
		return nil
	}
	message = modelMessageFromSDKParts(sdk.MessageRoleTool, results, message.Usage)
	return &message
}

func keepAskUserLegacyToolCalls(calls []conversation.ToolCall) []conversation.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	kept := make([]conversation.ToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.EqualFold(strings.TrimSpace(call.Function.Name), userinput.ToolNameAskUser) {
			kept = append(kept, call)
		}
	}
	return kept
}

func filterAssistantContextParts(parts []sdk.MessagePart) []sdk.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	kept := make([]sdk.MessagePart, 0, len(parts))
	for _, part := range parts {
		switch typed := part.(type) {
		case sdk.ToolCallPart:
			if strings.EqualFold(strings.TrimSpace(typed.ToolName), userinput.ToolNameAskUser) {
				kept = append(kept, typed)
			}
		case sdk.ToolResultPart, sdk.ReasoningPart:
			continue
		case sdk.TextPart:
			if strings.TrimSpace(typed.Text) != "" {
				kept = append(kept, typed)
			}
		default:
			kept = append(kept, part)
		}
	}
	return kept
}

func filterAskUserToolResults(parts []sdk.MessagePart) []sdk.MessagePart {
	if len(parts) == 0 {
		return nil
	}
	kept := make([]sdk.MessagePart, 0, len(parts))
	for _, part := range parts {
		result, ok := part.(sdk.ToolResultPart)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(result.ToolName), userinput.ToolNameAskUser) {
			kept = append(kept, result)
		}
	}
	return kept
}

func modelMessageFromSDKParts(role sdk.MessageRole, parts []sdk.MessagePart, usage json.RawMessage) conversation.ModelMessage {
	converted := sdkMessagesToModelMessages([]sdk.Message{{Role: role, Content: parts}})
	if len(converted) == 0 {
		return conversation.ModelMessage{Role: string(role)}
	}
	converted[0].Usage = usage
	return converted[0]
}
