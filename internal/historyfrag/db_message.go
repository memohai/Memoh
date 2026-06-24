package historyfrag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
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
		Namespace:   namespaceDBHistoryMessage,
		ID:          rowID,
		Version:     1,
		Schema:      contextfrag.SchemaContextRef,
		Durability:  contextfrag.RefDurable,
		HashAlgo:    contextfrag.HashAlgoSHA256,
		HashScope:   contextfrag.HashScopeSourcePayload,
		ContentHash: DBMessageSourceHash(msg).Value,
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
		Assets:       mediaRefsFromDBMessage(msg),

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

func DBMessageSourceHash(msg messagepkg.Message) contextfrag.FragmentHash {
	payload := dbMessageSourceHashPayload{
		ID:                      strings.TrimSpace(msg.ID),
		BotID:                   strings.TrimSpace(msg.BotID),
		SessionID:               strings.TrimSpace(msg.SessionID),
		SenderChannelIdentityID: strings.TrimSpace(msg.SenderChannelIdentityID),
		SenderUserID:            strings.TrimSpace(msg.SenderUserID),
		SenderDisplayName:       strings.TrimSpace(msg.SenderDisplayName),
		Platform:                strings.TrimSpace(msg.Platform),
		ExternalMessageID:       strings.TrimSpace(msg.ExternalMessageID),
		SourceReplyToMessageID:  strings.TrimSpace(msg.SourceReplyToMessageID),
		Role:                    strings.TrimSpace(msg.Role),
		Content:                 string(msg.Content),
		Usage:                   string(msg.Usage),
		EventID:                 strings.TrimSpace(msg.EventID),
		Assets:                  mediaRefsFromDBMessage(msg),
	}
	sort.Slice(payload.Assets, func(i, j int) bool {
		if payload.Assets[i].Ordinal != payload.Assets[j].Ordinal {
			return payload.Assets[i].Ordinal < payload.Assets[j].Ordinal
		}
		if payload.Assets[i].ContentHash != payload.Assets[j].ContentHash {
			return payload.Assets[i].ContentHash < payload.Assets[j].ContentHash
		}
		return payload.Assets[i].Role < payload.Assets[j].Role
	})
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return contextfrag.FragmentHash{
		Algo:  contextfrag.HashAlgoSHA256,
		Scope: contextfrag.HashScopeSourcePayload,
		Value: hex.EncodeToString(sum[:]),
	}
}

type dbMessageSourceHashPayload struct {
	ID                      string     `json:"id"`
	BotID                   string     `json:"bot_id,omitempty"`
	SessionID               string     `json:"session_id,omitempty"`
	SenderChannelIdentityID string     `json:"sender_channel_identity_id,omitempty"`
	SenderUserID            string     `json:"sender_user_id,omitempty"`
	SenderDisplayName       string     `json:"sender_display_name,omitempty"`
	Platform                string     `json:"platform,omitempty"`
	ExternalMessageID       string     `json:"external_message_id,omitempty"`
	SourceReplyToMessageID  string     `json:"source_reply_to_message_id,omitempty"`
	Role                    string     `json:"role"`
	Content                 string     `json:"content,omitempty"`
	Usage                   string     `json:"usage,omitempty"`
	EventID                 string     `json:"event_id,omitempty"`
	Assets                  []MediaRef `json:"assets,omitempty"`
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

func mediaRefsFromDBMessage(msg messagepkg.Message) []MediaRef {
	if len(msg.Assets) == 0 {
		return nil
	}
	out := make([]MediaRef, 0, len(msg.Assets))
	for _, asset := range msg.Assets {
		contentHash := strings.TrimSpace(asset.ContentHash)
		if contentHash == "" {
			continue
		}
		out = append(out, MediaRef{
			ContentHash: contentHash,
			Role:        strings.TrimSpace(asset.Role),
			Ordinal:     asset.Ordinal,
			Mime:        strings.TrimSpace(asset.Mime),
			SizeBytes:   asset.SizeBytes,
			Name:        strings.TrimSpace(asset.Name),
		})
	}
	return out
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
