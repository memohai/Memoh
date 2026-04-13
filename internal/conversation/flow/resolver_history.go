package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

type messageWithUsage struct {
	Message           conversation.ModelMessage
	UsageInputTokens  *int
	UsageOutputTokens *int
	SessionID         string
	ExternalMessageID string
	Platform          string
	SenderChannelID   string
	CompactID         string
}

func (r *Resolver) loadMessages(ctx context.Context, chatID string, sessionID string, maxContextMinutes int) ([]messageWithUsage, error) {
	if r.messageService == nil {
		return nil, nil
	}
	since := time.Now().UTC().Add(-time.Duration(maxContextMinutes) * time.Minute)
	var msgs []messagepkg.Message
	var err error
	if strings.TrimSpace(sessionID) != "" {
		msgs, err = r.messageService.ListActiveSinceBySession(ctx, sessionID, since)
	} else {
		msgs, err = r.messageService.ListActiveSince(ctx, chatID, since)
	}
	if err != nil {
		return nil, err
	}
	var result []messageWithUsage
	for _, m := range msgs {
		var mm conversation.ModelMessage
		if err := json.Unmarshal(m.Content, &mm); err != nil {
			r.logger.Warn("loadMessages: content unmarshal failed, treating as raw text",
				slog.String("chat_id", chatID), slog.Any("error", err))
			mm = conversation.ModelMessage{Role: m.Role, Content: m.Content}
		} else {
			mm.Role = m.Role
		}
		var inputTokens *int
		var outputTokens *int
		if len(m.Usage) > 0 {
			var u usageInfo
			if json.Unmarshal(m.Usage, &u) == nil {
				inputTokens = u.InputTokens
				outputTokens = u.OutputTokens
			}
		}
		result = append(result, messageWithUsage{
			Message:           mm,
			UsageInputTokens:  inputTokens,
			UsageOutputTokens: outputTokens,
			SessionID:         strings.TrimSpace(m.SessionID),
			ExternalMessageID: strings.TrimSpace(m.ExternalMessageID),
			Platform:          strings.TrimSpace(m.Platform),
			SenderChannelID:   strings.TrimSpace(m.SenderChannelIdentityID),
			CompactID:         strings.TrimSpace(m.CompactID),
		})
	}
	return result, nil
}

func dedupePersistedCurrentUserMessage(messages []messageWithUsage, req conversation.ChatRequest) []messageWithUsage {
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
		if !strings.EqualFold(strings.TrimSpace(item.Message.Role), "user") {
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
		if targetSenderChannelID != "" && item.SenderChannelID != "" && item.SenderChannelID != targetSenderChannelID {
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

func trimMessagesByTokens(log *slog.Logger, messages []messageWithUsage, maxTokens int) ([]conversation.ModelMessage, int) {
	if maxTokens == 0 || len(messages) == 0 {
		result := make([]conversation.ModelMessage, len(messages))
		for i, m := range messages {
			result[i] = m.Message
		}
		totalTokens := 0
		for _, m := range messages {
			totalTokens += estimateMessageTokens(m.Message)
		}
		return result, totalTokens
	}

	// Scan from newest to oldest, accumulating per-message estimated context
	// token costs. Each message's cost represents the tokens it occupies in the
	// context window (not the output tokens it generated). We use a character-
	// based estimate for all messages since this measures context window impact.
	totalTokens := 0
	cutoff := 0
	for i := len(messages) - 1; i >= 0; i-- {
		totalTokens += estimateMessageTokens(messages[i].Message)
		if totalTokens > maxTokens {
			cutoff = i + 1
			break
		}
	}

	// Keep provider-valid message order: a "tool" message must follow a preceding
	// assistant tool call. When history is head-trimmed, a leading tool message
	// may become orphaned and cause provider 400 errors.
	for cutoff < len(messages) && strings.EqualFold(strings.TrimSpace(messages[cutoff].Message.Role), "tool") {
		cutoff++
	}

	if cutoff > 0 && log != nil {
		log.Info("trimMessagesByTokens: context trimmed",
			slog.Int("total_messages", len(messages)),
			slog.Int("estimated_tokens", totalTokens),
			slog.Int("max_tokens", maxTokens),
			slog.Int("cutoff_index", cutoff),
			slog.Int("kept_messages", len(messages)-cutoff),
		)
	}

	result := make([]conversation.ModelMessage, 0, len(messages)-cutoff)
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
	for _, m := range messages[cutoff:] {
		result = append(result, m.Message)
	}
	return result, totalTokens
}

func (r *Resolver) replaceCompactedMessages(ctx context.Context, messages []messageWithUsage) []messageWithUsage {
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

	var result []messageWithUsage
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
		result = append(result, messageWithUsage{
			Message: conversation.ModelMessage{
				Role:    "user",
				Content: json.RawMessage(`"<summary>\n` + summary + `\n</summary>"`),
			},
		})
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
		contentJSON, err := json.Marshal(m.Content)
		if err != nil {
			continue
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
		if m.Role != "assistant" && m.Role != "tool" {
			continue
		}
		var mm conversation.ModelMessage
		if err := json.Unmarshal(m.Content, &mm); err != nil {
			continue
		}
		contentStr := ""
		if mm.Content != nil {
			contentStr = string(mm.Content)
		}
		trs = append(trs, pipelinepkg.TurnResponseEntry{
			RequestedAtMs: m.CreatedAt.UnixMilli(),
			Role:          m.Role,
			Content:       contentStr,
		})
	}
	return trs
}

// stripToolMessages removes tool messages and their associated assistant
// tool-call messages from the context. After synchronous compaction, the
// summary already captures the tool interactions — keeping raw tool output
// only wastes context tokens.
func stripToolMessages(messages []conversation.ModelMessage) []conversation.ModelMessage {
	filtered := make([]conversation.ModelMessage, 0, len(messages))
	for _, m := range messages {
		role := strings.TrimSpace(m.Role)
		if strings.EqualFold(role, "tool") {
			continue
		}
		// Remove assistant messages that contain tool calls (without text content).
		if strings.EqualFold(role, "assistant") && len(m.ToolCalls) > 0 {
			text := m.TextContent()
			if strings.TrimSpace(text) == "" {
				continue
			}
		}
		filtered = append(filtered, m)
	}
	return filtered
}
