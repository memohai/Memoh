// Package turn defines the application-level contract for starting and
// observing agent turns. It is the only surface Channel may depend on;
// it must not import Echo, fx, sqlc, or any channel package (guarded by
// internal/arch tests).
package turn

import (
	"context"
	"encoding/json"
)

// Mode selects the turn orchestration path.
type Mode string

const (
	ModeChat    Mode = "chat"
	ModeDiscuss Mode = "discuss"
)

// Attachment mirrors conversation.ChatAttachment as boundary-owned data.
type Attachment struct {
	Type        string         `json:"type"`
	Base64      string         `json:"base64,omitempty"`
	Path        string         `json:"path,omitempty"`
	URL         string         `json:"url,omitempty"`
	PlatformKey string         `json:"platform_key,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	Name        string         `json:"name,omitempty"`
	Mime        string         `json:"mime,omitempty"`
	Size        int64          `json:"size,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// SkillActivationSkill mirrors conversation.SkillActivationSkill.
type SkillActivationSkill struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	SourceKind  string `json:"source_kind,omitempty"`
	State       string `json:"state,omitempty"`
}

// SkillActivation mirrors conversation.SkillActivation.
type SkillActivation struct {
	Skills []SkillActivationSkill `json:"skills,omitempty"`
	Prompt string                 `json:"prompt,omitempty"`
}

// RequestedSkillContext mirrors conversation.RequestedSkillContext.
type RequestedSkillContext struct {
	Name           string
	Description    string
	Content        string
	SourceKind     string
	OpaqueSourceID string
	ContentHash    string
	Identity       string
}

// OutboundAssetRef mirrors conversation.OutboundAssetRef.
type OutboundAssetRef struct {
	ContentHash string
	Role        string
	Ordinal     int
	Mime        string
	SizeBytes   int64
	StorageKey  string
	Name        string
	Metadata    map[string]any
}

// InjectMessage carries a user message injected into a running turn
// between tool rounds.
type InjectMessage struct {
	Text            string
	Attachments     []Attachment
	HeaderifiedText string
}

// StartTurnCommand is a pure-data command. Field set mirrors exactly what
// the channel inbound processor supplies today (audited against
// conversation.ChatRequest); function- and channel-typed fields are
// intentionally excluded — injection goes through RunHandle.Inject and
// outbound assets through RunHandle.AddOutboundAssets.
type StartTurnCommand struct {
	SchemaVersion int
	TeamID        string // required; adapter fails closed when empty
	Mode          Mode

	BotID                   string
	ChatID                  string
	SessionID               string
	RouteID                 string
	Token                   string
	ChatToken               string
	UserID                  string
	SourceChannelIdentityID string
	DisplayName             string

	IdempotencyKey    string // derived from platform external message id
	ExternalMessageID string
	EventID           string

	Query           string
	ModelQuery      string
	UserMessageKind string
	UserVisibleText string
	Attachments     []Attachment

	ReplyTarget            string
	ConversationType       string
	ConversationName       string
	SourceReplyToMessageID string
	ReplySender            string
	ReplyPreview           string
	ReplyAttachments       []Attachment
	MentionsBot            bool
	RepliesToBot           bool

	ForwardMessageID          string
	ForwardFromUserID         string
	ForwardFromConversationID string
	ForwardSender             string
	ForwardDate               int64

	CurrentChannel string
	Channels       []string

	Model             string
	ReasoningEffort   string
	WorkspaceTargetID string

	SkillActivation      *SkillActivation
	RequestedSkills      []RequestedSkillContext
	SkipMemoryExtraction bool
	SkipTitleGeneration  bool
	UserMessagePersisted bool

	// Discuss-mode extras.
	SessionToken string //nolint:gosec // session credential material, in-process only
	ToolHTTPURL  string
}

// Event is one element of a turn's event stream. Payload is the raw JSON
// chunk exactly as produced by the runtime; Kind is the parsed "type"
// field (best effort, empty when unparsable). Seq is monotonically
// increasing per run.
type Event struct {
	RunID     string
	TeamID    string
	SessionID string
	Seq       int64
	Kind      string
	Payload   json.RawMessage
}

// RunHandle observes and steers one running turn. Events and Errs mirror
// the runtime's chunk/error channel pair; both close when the run ends.
type RunHandle interface {
	RunID() string
	Events() <-chan Event
	Errs() <-chan error
	Inject(ctx context.Context, msg InjectMessage) error
	AddOutboundAssets(refs []OutboundAssetRef)
	Cancel()
}

// Service starts turns. Implementations: inprocess (wraps flow.Resolver);
// a cross-process transport will implement the same contract later.
type Service interface {
	StartTurn(ctx context.Context, cmd StartTurnCommand) (RunHandle, error)
}
