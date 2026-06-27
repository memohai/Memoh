package flow

import (
	"context"
	"encoding/json"
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

func totalModelMessageTokens(messages []conversation.ModelMessage) int {
	total := 0
	for _, msg := range messages {
		total += estimateMessageTokens(msg)
	}
	return total
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
		totalTokens := totalHistoryTokens(messages)
		return historyfrag.ToModelMessages(messages), messages, totalTokens
	}

	retained, trimmed, totalTokens := selectHistoryRecordsForBudget(messages, maxTokens)
	if trimmed && log != nil {
		log.Info("trimMessagesByTokens: context trimmed",
			slog.Int("total_messages", len(messages)),
			slog.Int("estimated_tokens", totalTokens),
			slog.Int("max_tokens", maxTokens),
			slog.Int("kept_messages", len(retained)),
		)
	}

	result := make([]conversation.ModelMessage, 0, len(retained))
	if trimmed {
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
	retainedTokens := totalModelMessageTokens(result)
	return result, retained, retainedTokens
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
	if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
		return false
	}
	text := strings.TrimSpace(msg.TextContent())
	return strings.HasPrefix(text, "<summary>") || strings.HasPrefix(text, "[Conversation summary]")
}

func sameModelMessage(a conversation.ModelMessage, b conversation.ModelMessage) bool {
	if looksLikeSummaryMessage(a) && looksLikeSummaryMessage(b) {
		return normalizeSummaryText(a.TextContent()) == normalizeSummaryText(b.TextContent())
	}
	return strings.EqualFold(strings.TrimSpace(a.Role), strings.TrimSpace(b.Role)) &&
		string(a.Content) == string(b.Content)
}

func normalizeSummaryText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "[Conversation summary]")
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "<summary>")
	text = strings.TrimSuffix(text, "</summary>")
	return strings.TrimSpace(text)
}

func (r *Resolver) replaceCompactedMessages(ctx context.Context, sessionID string, scope contextfrag.Scope, messages []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
	if r.queries == nil {
		return messages
	}
	if strings.TrimSpace(sessionID) == "" {
		// Sessionless (chat-scoped) loads have no session log list to draw from;
		// resolve each in-window compact group individually.
		return r.replaceRecentCompactedMessages(ctx, messages)
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
	messages = replaceCompactedHistoryRecords(messages, summaries)
	sessionSummaries := r.summaryRecordsFromCompactionLogs(ctx, missingCompactionLogs(messages, logs), scope)
	if len(sessionSummaries) == 0 {
		return messages
	}
	return prependMissingCompactionSummaries(messages, sessionSummaries)
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

func (r *Resolver) replaceRecentCompactedMessages(ctx context.Context, messages []historyfrag.HistoryRecord) []historyfrag.HistoryRecord {
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
		if log.Status == "ok" && log.Summary != "" {
			summaries[compactID] = log.Summary
		}
	}

	return replaceCompactedHistoryRecords(messages, summaries)
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

func (r *Resolver) summaryRecordsFromCompactionLogs(ctx context.Context, logs []sqlc.BotHistoryMessageCompact, scope contextfrag.Scope) []historyfrag.HistoryRecord {
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
		coveredRefs := r.coveredRefsForCompact(ctx, log.ID, logScope)
		records = append(records, historyfrag.SummaryRecord(compactID, log.Summary, coveredRefs, logScope))
	}
	return records
}

func (r *Resolver) coveredRefsForCompact(ctx context.Context, compactID pgtype.UUID, scope contextfrag.Scope) []contextfrag.ContextRef {
	if r.queries == nil || !compactID.Valid {
		return nil
	}
	rows, err := r.queries.ListMessagesByCompactID(ctx, compactID)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("loadSessionCompactionSummaries: failed to load compacted message refs", slog.String("compact_id", pgUUIDString(compactID)), slog.Any("error", err))
		}
		return nil
	}
	refs := make([]contextfrag.ContextRef, 0, len(rows))
	for _, row := range rows {
		record, err := historyfrag.FromDBMessage(messageFromCompactRow(row), historyfrag.ScopeFallback{
			ConversationType: scope.ConversationType,
			ConversationName: scope.ConversationName,
			ReplyTarget:      scope.ReplyTarget,
		})
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("loadSessionCompactionSummaries: skipped compacted message ref", slog.String("compact_id", pgUUIDString(compactID)), slog.Any("error", err))
			}
			continue
		}
		refs = append(refs, record.Ref)
	}
	return refs
}

func messageFromCompactRow(row sqlc.ListMessagesByCompactIDRow) messagepkg.Message {
	return messagepkg.Message{
		ID:                      pgUUIDString(row.ID),
		BotID:                   pgUUIDString(row.BotID),
		SessionID:               pgUUIDString(row.SessionID),
		SenderChannelIdentityID: pgUUIDString(row.SenderChannelIdentityID),
		SenderUserID:            pgUUIDString(row.SenderUserID),
		ExternalMessageID:       pgTextString(row.ExternalMessageID),
		SourceReplyToMessageID:  pgTextString(row.SourceReplyToMessageID),
		Role:                    strings.TrimSpace(row.Role),
		Content:                 json.RawMessage(row.Content),
		Usage:                   json.RawMessage(row.Usage),
		CompactID:               pgUUIDString(row.CompactID),
		EventID:                 pgUUIDString(row.EventID),
		DisplayContent:          pgTextString(row.DisplayText),
		CreatedAt:               row.CreatedAt.Time,
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

func pgTextString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func replaceCompactedHistoryRecords(messages []historyfrag.HistoryRecord, summaries map[string]string) []historyfrag.HistoryRecord {
	compactGroups := make(map[string][]int)
	for i, m := range messages {
		if m.CompactID != "" {
			compactGroups[m.CompactID] = append(compactGroups[m.CompactID], i)
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
		summary, ok := summaries[m.CompactID]
		if !ok || summary == "" {
			for _, idx := range compactGroups[m.CompactID] {
				result = append(result, messages[idx])
			}
			continue
		}
		coveredRefs := make([]contextfrag.ContextRef, 0, len(compactGroups[m.CompactID]))
		for _, idx := range compactGroups[m.CompactID] {
			coveredRefs = append(coveredRefs, messages[idx].Ref)
		}
		result = append(result, historyfrag.SummaryRecord(m.CompactID, summary, coveredRefs, m.Scope))
	}
	return result
}

type pipelineContextBuild struct {
	Messages        []conversation.ModelMessage
	HistoryRecords  []historyfrag.HistoryRecord
	EstimatedTokens int
}

// buildPipelineContext assembles chat context from the DCP pipeline's
// RenderedContext (RC) merged with assistant/tool turns (TR) from
// bot_history_messages. This gives chat mode the same event-driven context
// that discuss mode uses, replacing the legacy loadMessages path.
func (r *Resolver) buildPipelineContext(ctx context.Context, req conversation.ChatRequest, contextTokenBudget int) pipelineContextBuild {
	sessionID := strings.TrimSpace(req.SessionID)
	if r.pipeline == nil || sessionID == "" {
		return pipelineContextBuild{}
	}
	rc := r.pipeline.GetRC(sessionID)

	historyMessages := r.loadPipelineHistoryMessages(ctx, sessionID)
	trs := pipelineTurnResponsesFromMessages(historyMessages)
	compactContext := r.loadPipelineCompactionContext(ctx, historyMessages)
	if contextTokenBudget > 0 {
		rc = appendCurrentQueryRenderedSegmentIfMissing(rc, req)
		rc, trs = pipelinepkg.TrimContextSourcesByBudget(rc, trs, compactContext.Summary, contextTokenBudget)
	}

	var messages []conversation.ModelMessage
	if composed := pipelinepkg.ComposeContextWithSummary(rc, trs, compactContext.Summary); composed != nil {
		messages = make([]conversation.ModelMessage, 0, len(composed.Messages))
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
	}
	messages = appendCurrentPipelineQueryIfMissing(messages, rc, req)

	estimatedTokens := 0
	for _, msg := range messages {
		estimatedTokens += estimateMessageTokens(msg)
	}
	return pipelineContextBuild{
		Messages:        messages,
		HistoryRecords:  compactContext.HistoryRecords,
		EstimatedTokens: estimatedTokens,
	}
}

func appendCurrentQueryRenderedSegmentIfMissing(rc pipelinepkg.RenderedContext, req conversation.ChatRequest) pipelinepkg.RenderedContext {
	query := strings.TrimSpace(firstNonEmpty(req.RawQuery, req.Query))
	currentMessageID := strings.TrimSpace(req.ExternalMessageID)
	if query == "" || currentMessageID == "" || renderedContextHasMessageID(rc, currentMessageID) {
		return rc
	}
	out := append(pipelinepkg.RenderedContext(nil), rc...)
	out = append(out, pipelinepkg.RenderedSegment{
		MessageID:    currentMessageID,
		ReceivedAtMs: latestRenderedContextEventMs(rc) + 1,
		Content:      []pipelinepkg.RenderedContentPiece{{Type: "text", Text: query}},
	})
	return out
}

func appendCurrentPipelineQueryIfMissing(messages []conversation.ModelMessage, rc pipelinepkg.RenderedContext, req conversation.ChatRequest) []conversation.ModelMessage {
	query := strings.TrimSpace(firstNonEmpty(req.RawQuery, req.Query))
	if query == "" {
		return messages
	}
	currentMessageID := strings.TrimSpace(req.ExternalMessageID)
	if len(rc) > 0 {
		if currentMessageID != "" && renderedContextHasMessageID(rc, currentMessageID) {
			return messages
		}
		if currentMessageID == "" && (renderedContextHasText(rc, query) || modelMessagesContainText(messages, query)) {
			return messages
		}
	}
	return append(messages, conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(query),
	})
}

func renderedContextHasMessageID(rc pipelinepkg.RenderedContext, messageID string) bool {
	if messageID == "" {
		return false
	}
	for _, seg := range rc {
		if strings.TrimSpace(seg.MessageID) == messageID {
			return true
		}
	}
	return false
}

func renderedContextHasText(rc pipelinepkg.RenderedContext, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, seg := range rc {
		for _, piece := range seg.Content {
			if strings.TrimSpace(piece.Text) == text {
				return true
			}
		}
	}
	return false
}

func modelMessagesContainText(messages []conversation.ModelMessage, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, msg := range messages {
		if strings.TrimSpace(msg.TextContent()) == text {
			return true
		}
	}
	return false
}

func latestRenderedContextEventMs(rc pipelinepkg.RenderedContext) int64 {
	var latest int64
	for _, seg := range rc {
		eventAt := seg.ReceivedAtMs
		if seg.LastEventAtMs > 0 {
			eventAt = seg.LastEventAtMs
		}
		if eventAt > latest {
			latest = eventAt
		}
	}
	return latest
}

func (r *Resolver) loadPipelineHistoryMessages(ctx context.Context, sessionID string) []messagepkg.Message {
	if r.messageService == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	msgs, err := r.messageService.ListActiveSinceBySession(ctx, sessionID, time.Unix(0, 0).UTC())
	if err != nil {
		r.logger.Warn("load pipeline history messages failed", slog.String("session_id", sessionID), slog.Any("error", err))
		return nil
	}
	return msgs
}

func (r *Resolver) LoadCompactionSummary(ctx context.Context, messages []messagepkg.Message) pipelinepkg.CompactSummary {
	return r.loadPipelineCompactionContext(ctx, messages).Summary
}

type pipelineCompactionContext struct {
	Summary        pipelinepkg.CompactSummary
	HistoryRecords []historyfrag.HistoryRecord
}

func (r *Resolver) loadPipelineCompactionContext(ctx context.Context, messages []messagepkg.Message) pipelineCompactionContext {
	if r.queries == nil || len(messages) == 0 {
		return pipelineCompactionContext{}
	}

	groups := make(map[string][]messagepkg.Message)
	recordGroups := make(map[string][]historyfrag.HistoryRecord)
	order := make([]string, 0)
	for _, msg := range messages {
		compactID := strings.TrimSpace(msg.CompactID)
		if compactID == "" {
			continue
		}
		if _, ok := groups[compactID]; !ok {
			order = append(order, compactID)
		}
		groups[compactID] = append(groups[compactID], msg)
		if record, err := historyfrag.FromDBMessage(msg, historyfrag.ScopeFallback{}); err == nil {
			recordGroups[compactID] = append(recordGroups[compactID], record)
		}
	}
	if len(order) == 0 {
		return pipelineCompactionContext{}
	}

	var summaries []string
	var summaryCompactIDs []string
	var summaryCoveredRefs []contextfrag.ContextRef
	seenCoveredRefs := make(map[string]struct{})
	var summaryScope contextfrag.Scope
	summaryScopeSet := false
	var coveredHistoryIDs []string
	var coveredMessageIDs []string
	coveredMessageCutoffMs := make(map[string]int64)
	for _, compactID := range order {
		cUUID, err := db.ParseUUID(compactID)
		if err != nil {
			continue
		}
		log, err := r.queries.GetCompactionLogByID(ctx, cUUID)
		if err != nil {
			r.logger.Warn("load pipeline compaction summary: failed to load compact log", slog.String("compact_id", compactID), slog.Any("error", err))
			continue
		}
		summary := strings.TrimSpace(log.Summary)
		if log.Status != "ok" || summary == "" {
			continue
		}
		summaries = append(summaries, summary)
		summaryCompactIDs = append(summaryCompactIDs, compactID)
		records := recordGroups[compactID]
		for _, record := range records {
			key := record.Ref.StableKey()
			if _, ok := seenCoveredRefs[key]; ok {
				continue
			}
			seenCoveredRefs[key] = struct{}{}
			summaryCoveredRefs = append(summaryCoveredRefs, record.Ref)
		}
		if !summaryScopeSet && len(records) > 0 {
			summaryScope = records[0].Scope
			summaryScopeSet = true
		}
		for _, msg := range groups[compactID] {
			if id := strings.TrimSpace(msg.ID); id != "" {
				coveredHistoryIDs = append(coveredHistoryIDs, id)
			}
			if externalID := strings.TrimSpace(msg.ExternalMessageID); externalID != "" {
				coveredMessageIDs = append(coveredMessageIDs, externalID)
				cutoffMs := msg.CreatedAt.UnixMilli()
				if cutoffMs > coveredMessageCutoffMs[externalID] {
					coveredMessageCutoffMs[externalID] = cutoffMs
				}
			}
		}
	}
	if len(summaries) == 0 {
		return pipelineCompactionContext{}
	}
	if len(coveredMessageCutoffMs) == 0 {
		coveredMessageCutoffMs = nil
	}
	summaryText := strings.Join(summaries, "\n\n")
	summaryRecordID := strings.Join(summaryCompactIDs, "+")
	return pipelineCompactionContext{
		Summary: pipelinepkg.CompactSummary{
			Text:                   summaryText,
			CoveredMessageIDs:      coveredMessageIDs,
			CoveredMessageCutoffMs: coveredMessageCutoffMs,

			CoveredHistoryMessageIDs: coveredHistoryIDs,
		},
		HistoryRecords: []historyfrag.HistoryRecord{
			historyfrag.SummaryRecord(summaryRecordID, summaryText, summaryCoveredRefs, summaryScope),
		},
	}
}

func pipelineTurnResponsesFromMessages(msgs []messagepkg.Message) []pipelinepkg.TurnResponseEntry {
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
