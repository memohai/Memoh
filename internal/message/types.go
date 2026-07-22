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
	RawMetadata             json.RawMessage `json:"-"`
	Usage                   json.RawMessage `json:"usage,omitempty"`
	SessionMode             string          `json:"session_mode,omitempty"`
	RuntimeType             string          `json:"runtime_type,omitempty"`
	Assets                  []MessageAsset  `json:"assets,omitempty"`
	CompactID               string          `json:"compact_id,omitempty"`
	EventID                 string          `json:"event_id,omitempty"`
	DisplayContent          string          `json:"display_content,omitempty"`
	TurnID                  string          `json:"turn_id,omitempty"`
	TurnPosition            int64           `json:"turn_position,omitempty"`
	TurnMessageSeq          int64           `json:"turn_message_seq,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
}

// RuntimeRowReservation is the durable identity assigned to one history row
// before that row is persisted. PersistRound must write these coordinates
// verbatim so runtime and history describe the same object.
type RuntimeRowReservation struct {
	MessageID      string `json:"message_id"`
	Role           string `json:"role"`
	TurnID         string `json:"turn_id"`
	TurnPosition   int64  `json:"turn_position"`
	TurnMessageSeq int64  `json:"turn_message_seq"`
}

// RuntimeTurnReservation reserves one globally ordered history turn. Gaps are
// intentional: an aborted run may consume a position without ever creating a
// history row, and that position must never be recycled.
type RuntimeTurnReservation struct {
	TurnID       string                `json:"turn_id"`
	TurnPosition int64                 `json:"turn_position"`
	Request      RuntimeRowReservation `json:"request"`
}

type HistoryTurn struct {
	ID                 string    `json:"id"`
	BotID              string    `json:"bot_id"`
	SessionID          string    `json:"session_id"`
	Position           int64     `json:"position"`
	RequestMessageID   string    `json:"request_message_id,omitempty"`
	AssistantMessageID string    `json:"assistant_message_id,omitempty"`
	SupersededByTurnID string    `json:"superseded_by_turn_id,omitempty"`
	SupersededAt       time.Time `json:"superseded_at,omitempty"`
	SupersededReason   string    `json:"superseded_reason,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
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
	MessageID               string
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
	SessionMode             string
	RuntimeType             string
	Assets                  []AssetRef
	ModelID                 string
	EventID                 string
	DisplayText             string
	TurnRequestMessageID    string
	TurnID                  string
	TurnPosition            int64
	TurnMessageSeq          int64
	SkipHistoryTurn         bool
}

type LocateResult struct {
	Messages []Message
	TargetID string
}

// Writer defines write behavior needed by the inbound router.
type Writer interface {
	Persist(ctx context.Context, input PersistInput) (Message, error)
}

// ToolTailRoundPersister optionally persists a complete
// user -> assistant(tool-call) -> tool -> assistant(final) round in one write.
type ToolTailRoundPersister interface {
	PersistToolTailRound(ctx context.Context, inputs []PersistInput) ([]Message, bool, error)
}

type TurnReplacement struct {
	OldTurnID        string
	RequestMessageID string
	TurnID           string
	TurnPosition     int64
	Reason           string
	SessionMetadata  map[string]any
}

type CanonicalTurn struct {
	ID               string
	BotID            string
	SessionID        string
	RequestMessageID string
}

type CanonicalTurnStart struct {
	Request     PersistInput
	Replacement *TurnReplacement
}

// CanonicalTurnPersister writes completed agent work directly into visible
// history. It does not model a durable run lifecycle.
type CanonicalTurnPersister interface {
	StartCanonicalTurn(ctx context.Context, start CanonicalTurnStart) (CanonicalTurn, Message, error)
	AppendCanonicalTurn(ctx context.Context, turn CanonicalTurn, inputs []PersistInput) ([]Message, error)
}

type RoundPersistenceOptions struct {
	Replacement *TurnReplacement
}

// AtomicRoundPersister writes a complete round in one transaction.
// Implementations must enforce any runtime fence carried by ctx, while still
// supporting unfenced local replacement transactions.
type AtomicRoundPersister interface {
	PersistRound(ctx context.Context, inputs []PersistInput, options RoundPersistenceOptions) ([]Message, bool, error)
}

// RuntimeTurnReserver allocates the turn order before an admitted run starts
// producing rows. Distributed callers carry a runtime fence in ctx.
type RuntimeTurnReserver interface {
	ReserveRuntimeTurn(ctx context.Context, botID, sessionID, requestMessageID string) (RuntimeTurnReservation, error)
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
	ListBeforeMessageBySession(ctx context.Context, sessionID string, beforeMessageID string, limit int32) ([]Message, error)
	LocateByExternalIDBySession(ctx context.Context, sessionID string, externalMessageID string, beforeLimit int32, afterLimit int32) (LocateResult, error)
	GetByIDBySession(ctx context.Context, sessionID string, messageID string) (Message, error)
	ListVisibleFromBySession(ctx context.Context, sessionID string, messageID string, maxCount int32) ([]Message, error)
	ListVisibleMessagesByTurnIDBySession(ctx context.Context, sessionID string, turnID string) ([]Message, error)
	GetVisibleTurnByMessage(ctx context.Context, sessionID string, messageID string) (HistoryTurn, error)
	GetLatestVisibleTurnBySession(ctx context.Context, sessionID string) (HistoryTurn, error)
	ReplaceTurn(ctx context.Context, sessionID string, oldTurnID string, requestMessageID string, assistantMessageID string, reason string) (HistoryTurn, error)
	DeleteByIDs(ctx context.Context, ids []string) error
	DeleteByBot(ctx context.Context, botID string) error
	DeleteBySession(ctx context.Context, sessionID string) error
	LinkAssets(ctx context.Context, messageID string, assets []AssetRef) error
}
