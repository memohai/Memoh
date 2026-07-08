package historyfrag

import (
	"time"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

const CollectorHistoryRecords = "history_records"

type SourceKind string

const (
	SourceDBMessage     SourceKind = "db_message"
	SourceCompactionLog SourceKind = "compaction_log"
)

type Lifecycle string

const (
	LifecyclePersisted     Lifecycle = "persisted"
	LifecycleLegacySummary Lifecycle = "legacy_summary"
)

type ScopeFallback struct {
	ChatID           string
	ConversationType string
	ConversationName string
	ReplyTarget      string
}

type HistoryRecord struct {
	Ref        contextfrag.ContextRef
	Kind       contextfrag.Kind
	SourceKind SourceKind
	Lifecycle  Lifecycle

	ModelMessage conversation.ModelMessage
	Assets       []MediaRef
	Metadata     map[string]any

	Scope      contextfrag.Scope
	Provenance contextfrag.Provenance

	DBMessageID       string
	ExternalMessageID string
	EventID           string
	SessionID         string
	BotID             string

	SenderChannelIdentityID string
	SenderUserID            string
	SenderDisplayName       string
	Platform                string
	SourceReplyToMessageID  string

	CompactID string
	CreatedAt time.Time

	UsageInputTokens  *int
	UsageOutputTokens *int

	// Required marks a record that must survive trimming/compaction because it
	// is pinned by a retry/edit request (conversation.ChatRequest.RequiredHistoryMessageID).
	Required bool
}

type MediaRef struct {
	ContentHash string
	Role        string
	Ordinal     int
	Name        string
	Metadata    map[string]any
}
