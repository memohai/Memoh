package message

import (
	"context"
	"encoding/json"
	"time"
)

// Message represents a single persisted bot message.
type Message struct {
	ID                      string          `json:"id"`
	BotID                   string          `json:"bot_id"`
	RouteID                 string          `json:"route_id,omitempty"`
	SenderChannelIdentityID string          `json:"sender_channel_identity_id,omitempty"`
	SenderUserID            string          `json:"sender_user_id,omitempty"`
	SenderDisplayName       string          `json:"sender_display_name,omitempty"`
	SenderAvatarURL         string          `json:"sender_avatar_url,omitempty"`
	Platform                string          `json:"platform,omitempty"`
	ExternalMessageID       string          `json:"external_message_id,omitempty"`
	SourceReplyToMessageID  string          `json:"source_reply_to_message_id,omitempty"`
	Role                    string          `json:"role"`
	Content                 json.RawMessage `json:"content"`
	Metadata                map[string]any  `json:"metadata,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
}

// PersistInput is the input for persisting a message.
type PersistInput struct {
	BotID                   string
	RouteID                 string
	SenderChannelIdentityID string
	SenderUserID            string
	Platform                string
	ExternalMessageID       string
	SourceReplyToMessageID  string
	Role                    string
	Content                 json.RawMessage
	Metadata                map[string]any
}

// Writer defines write behavior needed by the inbound router.
type Writer interface {
	Persist(ctx context.Context, input PersistInput) (Message, error)
}

// Service defines message read/write behavior.
type Service interface {
	Writer
	List(ctx context.Context, botID string) ([]Message, error)
	ListSince(ctx context.Context, botID string, since time.Time) ([]Message, error)
	ListLatest(ctx context.Context, botID string, limit int32) ([]Message, error)
	ListBefore(ctx context.Context, botID string, before time.Time, limit int32) ([]Message, error)
	DeleteByBot(ctx context.Context, botID string) error
}
