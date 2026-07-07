package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
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
	text := msg.TextContent()
	if len(text) == 0 {
		data, _ := json.Marshal(msg.Content)
		return len(data) / 4
	}
	return len(text) / 4
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
	retained := make([]historyfrag.HistoryRecord, 0, len(messages)-cutoff+len(requiredPrefix))
	retained = append(retained, requiredPrefix...)
	retained = append(retained, messages[cutoff:]...)
	result := make([]conversation.ModelMessage, 0, len(retained))
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
	for _, m := range retained {
		result = append(result, m.ModelMessage)
	}
	return result, retained, totalTokens
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

func historyContextFragsForMessages(messages []conversation.ModelMessage, records []historyfrag.HistoryRecord) []contextfrag.ContextFrag {
	if len(messages) == 0 || len(records) == 0 {
		return nil
	}
	frags := make([]contextfrag.ContextFrag, 0)
	recordStart := 0
	for i, msg := range messages {
		if !looksLikeSummaryMessage(msg) {
			continue
		}
		for j := recordStart; j < len(records); j++ {
			record := records[j]
			if record.Kind != contextfrag.KindConversationSummary || record.Coverage == nil {
				continue
			}
			if !sameModelMessage(record.ModelMessage, msg) {
				continue
			}
			frag := historyfrag.ToFrag(record)
			frag.ID = fmt.Sprintf("message.%03d", i)
			frag.Provenance.Index = i
			frags = append(frags, frag)
			recordStart = j + 1
			break
		}
	}
	return frags
}

func looksLikeSummaryMessage(msg conversation.ModelMessage) bool {
	return strings.EqualFold(strings.TrimSpace(msg.Role), "user") &&
		strings.HasPrefix(strings.TrimSpace(msg.TextContent()), "<summary>")
}

func sameModelMessage(a conversation.ModelMessage, b conversation.ModelMessage) bool {
	return strings.EqualFold(strings.TrimSpace(a.Role), strings.TrimSpace(b.Role)) &&
		string(a.Content) == string(b.Content)
}

func (r *Resolver) replaceCompactedMessages(ctx context.Context, sessionID string, scope contextfrag.Scope, messages []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	if r.queries == nil {
		return messages
	}
	if strings.TrimSpace(sessionID) == "" {
		// Sessionless (chat-scoped) loads have no session log list to draw from;
		// resolve each in-window compact group individually.
		return r.replaceRecentCompactedMessages(ctx, scope, messages)
	}
	logs := r.listSessionCompactionLogs(ctx, sessionID)
	if len(logs) == 0 {
		return messages
	}
	summaries := make(map[string]string, len(logs))
	for _, log := range logs {
		if log.Status != "ok" || strings.TrimSpace(log.Summary) == "" {
			continue
		}
		if id := pgUUIDString(log.ID); id != "" {
			summaries[id] = log.Summary
		}
	}
	messages = replaceCompactedHistoryRecords(messages, summaries, scope)
	sessionSummaries := summaryRecordsFromCompactionLogs(missingCompactionLogs(messages, logs), scope)
	if len(sessionSummaries) > 0 {
		messages = prependMissingCompactionSummaries(messages, sessionSummaries)
	}
	return r.refreshCompactedSummaryCoverage(ctx, messages)
}

// missingCompactionLogs filters the session logs down to those not already
// represented in the loaded records, so covered-ref lookups only run for
// summaries whose raw rows aged out of the load window.
func missingCompactionLogs(messages []historyfrag.HistoryRecord, logs []sqlc.BotHistoryMessageCompact) []sqlc.BotHistoryMessageCompact {
	seen := make(map[string]struct{}, len(messages))
	for _, record := range messages {
		if id := strings.TrimSpace(record.CompactID); id != "" {
			seen[id] = struct{}{}
		}
		if record.SourceKind == historyfrag.SourceCompactionLog {
			if id := strings.TrimSpace(record.Ref.ID); id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	missing := make([]sqlc.BotHistoryMessageCompact, 0, len(logs))
	for _, log := range logs {
		if _, ok := seen[pgUUIDString(log.ID)]; ok {
			continue
		}
		missing = append(missing, log)
	}
	return missing
}

func (r *Resolver) replaceRecentCompactedMessages(ctx context.Context, scope contextfrag.Scope, messages []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
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

	return r.refreshCompactedSummaryCoverage(ctx, replaceCompactedHistoryRecords(messages, summaries, scope))
}

func (r *Resolver) listSessionCompactionLogs(ctx context.Context, sessionID string) []sqlc.BotHistoryMessageCompact {
	sessionUUID, err := db.ParseUUID(sessionID)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("listSessionCompactionLogs: invalid session id", slog.String("session_id", sessionID), slog.Any("error", err))
		}
		return nil
	}
	logs, err := r.queries.ListCompactionLogsBySession(ctx, sessionUUID)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("listSessionCompactionLogs: failed to load compaction logs", slog.String("session_id", sessionID), slog.Any("error", err))
		}
		return nil
	}
	return logs
}

func summaryRecordsFromCompactionLogs(logs []sqlc.BotHistoryMessageCompact, scope contextfrag.Scope) []historyfrag.HistoryRecord {
	if len(logs) == 0 {
		return nil
	}
	records := make([]historyfrag.HistoryRecord, 0, len(logs))
	for _, log := range logs {
		if log.Status != "ok" || strings.TrimSpace(log.Summary) == "" {
			continue
		}
		compactID := pgUUIDString(log.ID)
		if compactID == "" {
			continue
		}
		logScope := scope
		if logScope.BotID == "" {
			logScope.BotID = pgUUIDString(log.BotID)
		}
		if logScope.SessionID == "" {
			logScope.SessionID = pgUUIDString(log.SessionID)
		}
		// Coverage is filled in afterward by refreshCompactedSummaryCoverage, once
		// per compact group, from the refs-only query.
		records = append(records, historyfrag.SummaryRecord(compactID, log.Summary, nil, logScope))
	}
	return records
}

// refreshCompactedSummaryCoverage recomputes each compaction summary's
// CoveredRefs from the full compact group via a refs-only query, so coverage
// is complete even when a group straddles the load window and no full-content
// row fetch is needed just to keep the refs.
func (r *Resolver) refreshCompactedSummaryCoverage(ctx context.Context, messages []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	if r.queries == nil {
		return messages
	}
	for i := range messages {
		record := &messages[i]
		if record.SourceKind != historyfrag.SourceCompactionLog || record.Kind != contextfrag.KindConversationSummary {
			continue
		}
		compactID, err := db.ParseUUID(record.CompactID)
		if err != nil {
			continue
		}
		coverage := contextfrag.NewSummaryCoverage(record.Ref, r.coveredRefsForCompact(ctx, compactID))
		record.Coverage = &coverage
	}
	return messages
}

func (r *Resolver) coveredRefsForCompact(ctx context.Context, compactID pgtype.UUID) []contextfrag.ContextRef {
	if r.queries == nil || !compactID.Valid {
		return nil
	}
	rows, err := r.queries.ListMessageRefsByCompactID(ctx, compactID)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("coveredRefsForCompact: failed to load compacted message refs", slog.String("compact_id", pgUUIDString(compactID)), slog.Any("error", err))
		}
		return nil
	}
	refs := make([]contextfrag.ContextRef, 0, len(rows))
	for _, row := range rows {
		record, err := historyfrag.FromDBMessage(messageFromCompactRefRow(row), historyfrag.ScopeFallback{})
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("coveredRefsForCompact: skipped compacted message ref", slog.String("compact_id", pgUUIDString(compactID)), slog.Any("error", err))
			}
			continue
		}
		refs = append(refs, record.Ref)
	}
	return refs
}

func messageFromCompactRefRow(row sqlc.ListMessageRefsByCompactIDRow) messagepkg.Message {
	return messagepkg.Message{
		ID:        pgUUIDString(row.ID),
		BotID:     pgUUIDString(row.BotID),
		SessionID: pgUUIDString(row.SessionID),
	}
}

func prependMissingCompactionSummaries(messages []historyfrag.HistoryRecord, summaries []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	if len(summaries) == 0 {
		return messages
	}
	seen := make(map[string]struct{}, len(messages))
	for _, record := range messages {
		if id := strings.TrimSpace(record.CompactID); id != "" {
			seen[id] = struct{}{}
		}
		if record.SourceKind == historyfrag.SourceCompactionLog {
			if id := strings.TrimSpace(record.Ref.ID); id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	missing := make([]historyfrag.HistoryRecord, 0, len(summaries))
	for _, summary := range summaries {
		id := strings.TrimSpace(summary.CompactID)
		if id == "" {
			id = strings.TrimSpace(summary.Ref.ID)
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		missing = append(missing, summary)
	}
	if len(missing) == 0 {
		return messages
	}
	out := make([]historyfrag.HistoryRecord, 0, len(missing)+len(messages))
	out = append(out, missing...)
	out = append(out, messages...)
	return out
}

func pgUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
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
		coveredRefs := make([]contextfrag.ContextRef, 0, len(compactGroups[m.CompactID]))
		for _, idx := range compactGroups[m.CompactID] {
			coveredRefs = append(coveredRefs, messages[idx].Ref)
		}
		result = append(result, historyfrag.SummaryRecord(m.CompactID, summary, coveredRefs, scope))
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
