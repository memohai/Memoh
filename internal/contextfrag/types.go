// Package contextfrag defines the typed intermediate representation used to
// describe context before it is rendered into provider-specific SDK inputs.
package contextfrag

import sdk "github.com/memohai/twilight-ai/sdk"

// Kind identifies the semantic source and intent of a context fragment.
type Kind string

const (
	KindSystemPrompt         Kind = "system_prompt"
	KindSystemPolicy         Kind = "system_policy"
	KindBotIdentity          Kind = "bot_identity"
	KindWorkspaceInstruction Kind = "workspace_instruction"
	KindPlatformIdentity     Kind = "platform_identity"
	KindToolUsage            Kind = "tool_usage"
	KindConversationEvent    Kind = "conversation_event"
	KindCurrentUserMessage   Kind = "current_user_message"
	KindAttachmentRef        Kind = "attachment_ref"
	KindNativeImage          Kind = "native_image"
	KindHookContext          Kind = "hook_context"
	KindBackgroundSummary    Kind = "background_summary"
	KindACPContext           Kind = "acp_context"

	// Reserved for the memory/compaction rewrites. Phase 1 keeps their existing
	// resolver paths intact while making room for future collectors.
	KindMemoryRecall        Kind = "memory_recall"
	KindConversationSummary Kind = "conversation_summary"
)

// Slot describes where a fragment is rendered in the LLM input layout.
type Slot string

const (
	SlotSystem                    Slot = "system"
	SlotBeforeHistory             Slot = "before_history"
	SlotHistory                   Slot = "history"
	SlotAfterHistoryBeforeCurrent Slot = "after_history_before_current"
	SlotCurrentUser               Slot = "current_user"
	SlotAfterCurrent              Slot = "after_current"
)

// CacheClass marks whether a fragment is expected to be prompt-cache friendly.
type CacheClass string

const (
	CacheStable  CacheClass = "stable"
	CacheDynamic CacheClass = "dynamic"
	CacheNever   CacheClass = "never"
)

// TrustLevel records whether a fragment comes from Memoh-controlled state or
// untrusted external conversation content.
type TrustLevel string

const (
	TrustSystem    TrustLevel = "system"
	TrustWorkspace TrustLevel = "workspace"
	TrustUser      TrustLevel = "user"
	TrustExternal  TrustLevel = "external"
)

// OverflowAction is the policy to use when a fragment exceeds budget.
type OverflowAction string

const (
	OverflowKeep      OverflowAction = "keep"
	OverflowTrim      OverflowAction = "trim"
	OverflowSummarize OverflowAction = "summarize"
	OverflowDrop      OverflowAction = "drop"
)

// BudgetPolicy captures the planned budget behavior for a fragment. Phase 1
// records policy only; enforcement remains in the existing trimming paths.
type BudgetPolicy struct {
	MaxTokens int            `json:"max_tokens,omitempty"`
	MaxChars  int            `json:"max_chars,omitempty"`
	Overflow  OverflowAction `json:"overflow,omitempty"`
}

// RenderFormat describes how a fragment should be rendered.
type RenderFormat string

const (
	RenderPlainText  RenderFormat = "plain_text"
	RenderMarkdown   RenderFormat = "markdown"
	RenderSDKMessage RenderFormat = "sdk_message"
	RenderNativePart RenderFormat = "native_part"
)

// RenderPolicy stores rendering hints. Anchor is used for sections such as
// tool usage that must land before a known heading.
type RenderPolicy struct {
	Format RenderFormat `json:"format,omitempty"`
	Anchor string       `json:"anchor,omitempty"`
}

// Provenance identifies where a fragment came from.
type Provenance struct {
	Source    string `json:"source,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
	Collector string `json:"collector,omitempty"`
	Index     int    `json:"index,omitempty"`
}

// AttentionReason explains why an IM/group-chat event deserves attention.
type AttentionReason string

const (
	AttentionDirect    AttentionReason = "direct"
	AttentionMention   AttentionReason = "mention"
	AttentionReply     AttentionReason = "reply"
	AttentionCommand   AttentionReason = "command"
	AttentionSchedule  AttentionReason = "schedule"
	AttentionHeartbeat AttentionReason = "heartbeat"
	AttentionPassive   AttentionReason = "passive"
)

// Scope preserves IM/group-chat topology separately from rendered text.
type Scope struct {
	BotID                     string            `json:"bot_id,omitempty"`
	ChatID                    string            `json:"chat_id,omitempty"`
	SessionID                 string            `json:"session_id,omitempty"`
	ChannelIdentityID         string            `json:"channel_identity_id,omitempty"`
	DisplayName               string            `json:"display_name,omitempty"`
	Platform                  string            `json:"platform,omitempty"`
	ConversationType          string            `json:"conversation_type,omitempty"`
	ConversationName          string            `json:"conversation_name,omitempty"`
	ReplyTarget               string            `json:"reply_target,omitempty"`
	CurrentMessageID          string            `json:"current_message_id,omitempty"`
	EventID                   string            `json:"event_id,omitempty"`
	ReplyToMessageID          string            `json:"reply_to_message_id,omitempty"`
	ReplySender               string            `json:"reply_sender,omitempty"`
	MentionsBot               bool              `json:"mentions_bot,omitempty"`
	RepliesToBot              bool              `json:"replies_to_bot,omitempty"`
	ForwardMessageID          string            `json:"forward_message_id,omitempty"`
	ForwardFromUserID         string            `json:"forward_from_user_id,omitempty"`
	ForwardFromConversationID string            `json:"forward_from_conversation_id,omitempty"`
	Attention                 []AttentionReason `json:"attention,omitempty"`
	Metadata                  map[string]string `json:"metadata,omitempty"`
}

// PartType identifies the payload shape inside a ContextFrag.
type PartType string

const (
	PartText       PartType = "text"
	PartSDKMessage PartType = "sdk_message"
	PartImage      PartType = "image"
)

// ImageRef records image metadata without embedding the image payload in the
// manifest. The actual SDK image part is retained in SDKImage for rendering.
type ImageRef struct {
	MediaType string `json:"media_type,omitempty"`
	Source    string `json:"source,omitempty"`
}

// Part is one payload item in a context fragment.
type Part struct {
	Type       PartType       `json:"type"`
	Text       string         `json:"text,omitempty"`
	Image      ImageRef       `json:"image,omitempty"`
	SDKMessage *sdk.Message   `json:"-"`
	SDKImage   *sdk.ImagePart `json:"-"`
}

// ContextFrag is the typed context fragment abstraction.
type ContextFrag struct {
	ID         string          `json:"id"`
	Kind       Kind            `json:"kind"`
	Role       sdk.MessageRole `json:"role,omitempty"`
	Slot       Slot            `json:"slot"`
	Priority   int             `json:"priority,omitempty"`
	CacheClass CacheClass      `json:"cache_class,omitempty"`
	Trust      TrustLevel      `json:"trust,omitempty"`
	Scope      Scope           `json:"scope,omitempty"`
	Budget     BudgetPolicy    `json:"budget,omitempty"`
	Render     RenderPolicy    `json:"render,omitempty"`
	Provenance Provenance      `json:"provenance,omitempty"`
	Parts      []Part          `json:"parts,omitempty"`
}

// AssembledContext is the compiled view produced from fragments.
type AssembledContext struct {
	Frags        []ContextFrag   `json:"frags,omitempty"`
	System       string          `json:"system,omitempty"`
	Messages     []sdk.Message   `json:"-"`
	Query        string          `json:"query,omitempty"`
	InlineImages []sdk.ImagePart `json:"-"`
	Manifest     Manifest        `json:"manifest"`
}

// Manifest is a content-light accounting view for debugging and review.
type Manifest struct {
	Version         int              `json:"version"`
	View            ManifestView     `json:"view,omitempty"`
	DynamicMutators []DynamicMutator `json:"dynamic_mutators,omitempty"`
	Counts          ManifestCounts   `json:"counts"`
	Items           []ManifestItem   `json:"items,omitempty"`
}

// ManifestView names the exact view represented by a manifest.
type ManifestView string

const (
	ViewRunConfigPreProvider ManifestView = "run_config_pre_provider"
)

// DynamicMutator names a later runtime transform that can change provider params
// after the RunConfig-level context frag view has been compiled.
type DynamicMutator string

const (
	DynamicMutatorPromptCache         DynamicMutator = "prompt_cache"
	DynamicMutatorInjectCh            DynamicMutator = "inject_ch"
	DynamicMutatorReadMedia           DynamicMutator = "read_media"
	DynamicMutatorBeforeModelCallHook DynamicMutator = "before_model_call_hook"
	DynamicMutatorBackgroundSummary   DynamicMutator = "background_summary"
	DynamicMutatorMidTaskPrune        DynamicMutator = "mid_task_prune"
)

// ManifestCounts summarizes fragment composition.
type ManifestCounts struct {
	Fragments int `json:"fragments"`
	Messages  int `json:"messages"`
	Images    int `json:"images"`
	TextBytes int `json:"text_bytes"`
}

// ManifestItem is one non-sensitive fragment entry.
type ManifestItem struct {
	ID         string          `json:"id"`
	Kind       Kind            `json:"kind"`
	Slot       Slot            `json:"slot"`
	Role       sdk.MessageRole `json:"role,omitempty"`
	Priority   int             `json:"priority,omitempty"`
	CacheClass CacheClass      `json:"cache_class,omitempty"`
	Trust      TrustLevel      `json:"trust,omitempty"`
	Source     string          `json:"source,omitempty"`
	SourceID   string          `json:"source_id,omitempty"`
	Collector  string          `json:"collector,omitempty"`
	PartTypes  []PartType      `json:"part_types,omitempty"`
	TextBytes  int             `json:"text_bytes,omitempty"`
	ImageCount int             `json:"image_count,omitempty"`
	Scope      Scope           `json:"scope,omitempty"`
}
