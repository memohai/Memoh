package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type messageWithUsage struct {
	Message           conversation.ModelMessage
	UsageInputTokens  *int
	UsageOutputTokens *int
	SessionID         string
	ExternalMessageID string
	Platform          string
	SenderChannelID   string
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

func trimMessagesByTokens(log *slog.Logger, messages []messageWithUsage, maxTokens int) []conversation.ModelMessage {
	if maxTokens == 0 || len(messages) == 0 {
		result := make([]conversation.ModelMessage, len(messages))
		for i, m := range messages {
			result[i] = m.Message
		}
		return result
	}

	// Scan from newest to oldest, accumulating per-message token costs.
	// Messages with stored usage data use that value; others fall back to a
	// character-based estimate so that user/tool messages are not free-passed.
	totalTokens := 0
	cutoff := 0
	messagesWithUsage := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].UsageOutputTokens != nil {
			totalTokens += *messages[i].UsageOutputTokens
			messagesWithUsage++
		} else {
			totalTokens += estimateMessageTokens(messages[i].Message)
		}
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

	if log != nil {
		log.Debug("trimMessagesByTokens",
			slog.Int("total_messages", len(messages)),
			slog.Int("messages_with_usage", messagesWithUsage),
			slog.Int("accumulated_output_tokens", totalTokens),
			slog.Int("max_tokens", maxTokens),
			slog.Int("cutoff_index", cutoff),
			slog.Int("kept_messages", len(messages)-cutoff),
		)
	}

	result := make([]conversation.ModelMessage, 0, len(messages)-cutoff)
	for _, m := range messages[cutoff:] {
		result = append(result, m.Message)
	}
	return result
}
