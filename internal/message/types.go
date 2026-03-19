package message

import (
	"context"
	"encoding/json"
	"time"
)

// MessageAsset carries media asset metadata attached to a message.
// ContentHash is the content-addressed identifier for the media file.
type MessageAsset struct {
	ContentHash string         `json:"content_hash"`
	Role        string         `json:"role"`
	Ordinal     int            `json:"ordinal"`
	Mime        string         `json:"mime"`
	SizeBytes   int64          `json:"size_bytes"`
	StorageKey  string         `json:"storage_key"`
	Name        string         `json:"name,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Message represents a single persisted bot message.
type Message struct {
	ID                      string          `json:"id"`
	BotID                   string          `json:"bot_id"`
	SessionID               string          `json:"session_id,omitempty"`
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
	Usage                   json.RawMessage `json:"usage,omitempty"`
	Assets                  []MessageAsset  `json:"assets,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
}

// AssetRef links a media asset to a persisted message.
// ContentHash is the content-addressed identifier for the media file.
type AssetRef struct {
	ContentHash string         `json:"content_hash"`
	Role        string         `json:"role"`
	Ordinal     int            `json:"ordinal"`
	Mime        string         `json:"mime,omitempty"`
	SizeBytes   int64          `json:"size_bytes,omitempty"`
	StorageKey  string         `json:"storage_key,omitempty"`
	Name        string         `json:"name,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// PersistInput is the input for persisting a message.
type PersistInput struct {
	BotID                   string
	SessionID               string
	SenderChannelIdentityID string
	SenderUserID            string
	ExternalMessageID       string
	SourceReplyToMessageID  string
	Role                    string
	Content                 json.RawMessage
	Metadata                map[string]any
	Usage                   json.RawMessage
	Assets                  []AssetRef
	ModelID                 string
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
	ListActiveSince(ctx context.Context, botID string, since time.Time) ([]Message, error)
	ListLatest(ctx context.Context, botID string, limit int32) ([]Message, error)
	ListBefore(ctx context.Context, botID string, before time.Time, limit int32) ([]Message, error)
	ListBySession(ctx context.Context, sessionID string) ([]Message, error)
	ListSinceBySession(ctx context.Context, sessionID string, since time.Time) ([]Message, error)
	ListActiveSinceBySession(ctx context.Context, sessionID string, since time.Time) ([]Message, error)
	ListLatestBySession(ctx context.Context, sessionID string, limit int32) ([]Message, error)
	ListBeforeBySession(ctx context.Context, sessionID string, before time.Time, limit int32) ([]Message, error)
	DeleteByBot(ctx context.Context, botID string) error
	DeleteBySession(ctx context.Context, sessionID string) error
	LinkAssets(ctx context.Context, messageID string, assets []AssetRef) error
}
