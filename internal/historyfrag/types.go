package historyfrag

import (
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

const CollectorHistoryRecords = "history_records"

type SourceKind string

const (
	SourceDBMessage     SourceKind = "db_message"
	SourceCompactionLog SourceKind = "compaction_log"
	SourcePipelineRC    SourceKind = "pipeline_rc"
	SourcePipelineTR    SourceKind = "pipeline_tr"
)

type Lifecycle string

const (
	LifecyclePersisted       Lifecycle = "persisted"
	LifecycleLegacySummary   Lifecycle = "legacy_summary"
	LifecycleRuntimeInjected Lifecycle = "runtime_injected"
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
	SDKMessage   sdk.Message

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
}
