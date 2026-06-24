package historyfrag

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

const namespaceDBHistoryMessage = "bot_history_message"

type usageInfo struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
}

func FromDBMessage(msg messagepkg.Message, fallback ScopeFallback) (HistoryRecord, error) {
	rowID := strings.TrimSpace(msg.ID)
	if rowID == "" {
		return HistoryRecord{}, errors.New("history db message id is required")
	}

	modelMessage := modelMessageFromDBMessage(msg)
	inputTokens, outputTokens := parseUsage(msg.Usage)
	ref := contextfrag.ContextRef{
		Namespace:  namespaceDBHistoryMessage,
		ID:         rowID,
		Version:    1,
		Schema:     contextfrag.SchemaContextRef,
		Durability: contextfrag.RefDurable,
	}
	scope := scopeFromDBMessage(msg, fallback)
	provenance := contextfrag.Provenance{
		Source:    string(SourceDBMessage),
		SourceID:  rowID,
		Collector: CollectorHistoryRecords,
	}

	return HistoryRecord{
		Ref:        ref,
		Kind:       contextfrag.KindConversationEvent,
		SourceKind: SourceDBMessage,
		Lifecycle:  LifecyclePersisted,

		ModelMessage: modelMessage,

		Scope:      scope,
		Provenance: provenance,

		DBMessageID:       rowID,
		ExternalMessageID: strings.TrimSpace(msg.ExternalMessageID),
		EventID:           strings.TrimSpace(msg.EventID),
		SessionID:         strings.TrimSpace(msg.SessionID),
		BotID:             strings.TrimSpace(msg.BotID),

		SenderChannelIdentityID: strings.TrimSpace(msg.SenderChannelIdentityID),
		SenderUserID:            strings.TrimSpace(msg.SenderUserID),
		SenderDisplayName:       strings.TrimSpace(msg.SenderDisplayName),
		Platform:                strings.TrimSpace(msg.Platform),
		SourceReplyToMessageID:  strings.TrimSpace(msg.SourceReplyToMessageID),

		CompactID: strings.TrimSpace(msg.CompactID),
		CreatedAt: msg.CreatedAt,

		UsageInputTokens:  inputTokens,
		UsageOutputTokens: outputTokens,
	}, nil
}

func modelMessageFromDBMessage(msg messagepkg.Message) conversation.ModelMessage {
	var modelMessage conversation.ModelMessage
	if err := json.Unmarshal(msg.Content, &modelMessage); err != nil {
		modelMessage = conversation.ModelMessage{
			Role:    strings.TrimSpace(msg.Role),
			Content: cloneRawMessage(msg.Content),
		}
	} else {
		modelMessage.Role = strings.TrimSpace(msg.Role)
	}
	return modelMessage
}

func parseUsage(raw json.RawMessage) (*int, *int) {
	if len(raw) == 0 {
		return nil, nil
	}
	var usage usageInfo
	if err := json.Unmarshal(raw, &usage); err != nil {
		return nil, nil
	}
	return usage.InputTokens, usage.OutputTokens
}

func scopeFromDBMessage(msg messagepkg.Message, fallback ScopeFallback) contextfrag.Scope {
	return contextfrag.Scope{
		BotID:             strings.TrimSpace(msg.BotID),
		ChatID:            strings.TrimSpace(fallback.ChatID),
		SessionID:         strings.TrimSpace(msg.SessionID),
		ChannelIdentityID: strings.TrimSpace(msg.SenderChannelIdentityID),
		DisplayName:       strings.TrimSpace(msg.SenderDisplayName),
		Platform:          strings.TrimSpace(msg.Platform),
		ConversationType:  strings.TrimSpace(fallback.ConversationType),
		ConversationName:  strings.TrimSpace(fallback.ConversationName),
		ReplyTarget:       strings.TrimSpace(fallback.ReplyTarget),
		CurrentMessageID:  strings.TrimSpace(msg.ExternalMessageID),
		EventID:           strings.TrimSpace(msg.EventID),
		ReplyToMessageID:  strings.TrimSpace(msg.SourceReplyToMessageID),
	}
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
