package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

type pipelineContextBuild struct {
	Messages        []conversation.ModelMessage
	HistoryRecords  []historyfrag.HistoryRecord
	EstimatedTokens int
}

type composedPipelineMessage struct {
	message       conversation.ModelMessage
	summaryRecord historyfrag.HistoryRecord
	hasSummary    bool
}

func (r *Resolver) buildPipelineContext(
	ctx context.Context,
	req conversation.ChatRequest,
	contextTokenBudget int,
) (pipelineContextBuild, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if r.pipeline == nil || sessionID == "" {
		return pipelineContextBuild{}, nil
	}
	rc := r.pipeline.GetRC(sessionID)
	historyMessages, records, err := r.loadPipelineHistorySnapshot(ctx, req)
	if err != nil {
		return pipelineContextBuild{}, err
	}
	scope := compactionSummaryScope(
		req.BotID,
		req.ChatID,
		req.SessionID,
		req.ConversationType,
		req.ConversationName,
		req.ReplyTarget,
	)
	artifacts, summaryRecords, err := r.loadPipelineCompactionArtifacts(ctx, scope, records)
	if err != nil {
		return pipelineContextBuild{}, err
	}
	trs := pipelineTurnResponses(historyMessages)
	composed := pipelinepkg.ComposeContextWithArtifacts(rc, trs, artifacts)
	entries := composedPipelineMessages(composed, summaryRecords)
	entries = appendCurrentPipelineQueryIfMissing(entries, pipelinepkg.ActiveRenderedContext(rc, artifacts), req)
	return trimComposedPipelineMessages(r.logger, entries, contextTokenBudget), nil
}

func (r *Resolver) loadPipelineHistorySnapshot(
	ctx context.Context,
	req conversation.ChatRequest,
) ([]messagepkg.Message, []historyfrag.HistoryRecord, error) {
	if r.messageService == nil {
		return nil, nil, nil
	}
	messages, err := r.messageService.ListActiveSinceBySession(ctx, req.SessionID, time.Unix(0, 0).UTC())
	if err != nil {
		return nil, nil, err
	}
	fallback := historyScopeFallbackFromChatRequest(req)
	records := make([]historyfrag.HistoryRecord, 0, len(messages))
	for _, message := range messages {
		record, err := historyfrag.FromDBMessageWithLogger(r.logger, message, fallback)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, record)
	}
	return messages, records, nil
}

func pipelineTurnResponses(messages []messagepkg.Message) []pipelinepkg.TurnResponseEntry {
	responses := make([]pipelinepkg.TurnResponseEntry, 0, len(messages))
	for _, message := range messages {
		entry, ok := pipelinepkg.DecodeTurnResponseEntry(message)
		if ok {
			responses = append(responses, entry)
		}
	}
	return responses
}

func composedPipelineMessages(
	composed *pipelinepkg.ComposeContextResult,
	summaryRecords []historyfrag.HistoryRecord,
) []composedPipelineMessage {
	if composed == nil {
		return nil
	}
	summaries := make(map[string]historyfrag.HistoryRecord, len(summaryRecords))
	for _, record := range summaryRecords {
		summaries[record.Ref.ID] = record
	}
	entries := make([]composedPipelineMessage, 0, len(composed.Messages))
	for _, message := range composed.Messages {
		content := message.RawContent
		if len(content) == 0 {
			content, _ = json.Marshal(message.Content)
		}
		entry := composedPipelineMessage{message: conversation.ModelMessage{Role: message.Role, Content: content}}
		if summary, ok := summaries[message.CompactionArtifactID]; ok {
			entry.summaryRecord = summary
			entry.hasSummary = true
		}
		entries = append(entries, entry)
	}
	return entries
}

func appendCurrentPipelineQueryIfMissing(
	entries []composedPipelineMessage,
	rc pipelinepkg.RenderedContext,
	req conversation.ChatRequest,
) []composedPipelineMessage {
	query := strings.TrimSpace(firstNonEmpty(req.RawQuery, req.Query))
	if query == "" {
		return entries
	}
	currentMessageID := strings.TrimSpace(req.ExternalMessageID)
	if currentMessageID != "" && renderedContextHasMessageID(rc, currentMessageID) {
		return entries
	}
	if currentMessageID == "" && pipelineEntriesContainText(entries, query) {
		return entries
	}
	return append(entries, composedPipelineMessage{message: conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(query),
	}})
}

func renderedContextHasMessageID(rc pipelinepkg.RenderedContext, messageID string) bool {
	for _, segment := range rc {
		if strings.TrimSpace(segment.MessageID) == messageID {
			return true
		}
	}
	return false
}

func pipelineEntriesContainText(entries []composedPipelineMessage, text string) bool {
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.message.Role), "user") &&
			strings.TrimSpace(entry.message.TextContent()) == text {
			return true
		}
	}
	return false
}

func trimComposedPipelineMessages(
	log *slog.Logger,
	entries []composedPipelineMessage,
	maxTokens int,
) pipelineContextBuild {
	cutoff := 0
	if maxTokens > 0 {
		tokens := 0
		for i := len(entries) - 1; i >= 0; i-- {
			tokens += estimateMessageTokens(entries[i].message)
			if tokens > maxTokens {
				cutoff = i + 1
				break
			}
		}
		for cutoff < len(entries) && strings.EqualFold(strings.TrimSpace(entries[cutoff].message.Role), "tool") {
			cutoff++
		}
	}
	if cutoff > 0 && log != nil {
		log.Info("trim pipeline context",
			slog.Int("total_messages", len(entries)),
			slog.Int("max_tokens", maxTokens),
			slog.Int("kept_messages", len(entries)-cutoff),
		)
	}
	retained := entries[cutoff:]
	build := pipelineContextBuild{
		Messages:       make([]conversation.ModelMessage, 0, len(retained)),
		HistoryRecords: make([]historyfrag.HistoryRecord, 0),
	}
	for _, entry := range retained {
		build.Messages = append(build.Messages, entry.message)
		build.EstimatedTokens += estimateMessageTokens(entry.message)
		if entry.hasSummary {
			build.HistoryRecords = append(build.HistoryRecords, entry.summaryRecord)
		}
	}
	return build
}
