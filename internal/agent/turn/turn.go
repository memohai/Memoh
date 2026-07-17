// Package turn defines the application-level contract for starting and
// observing agent turns. It is the only agent surface Channel may depend
// on; it must not import Echo, fx, sqlc, or any channel package (guarded
// by internal/arch tests).
//
// Data-carrier types are currently aliases of the conversation package's
// wire shapes: consumers decode the same bytes either way, and the alias
// keeps this migration behavior-preserving. When a cross-process transport
// versions the payload format these aliases become owned types.
package turn

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/userinput"
)

// ErrDuplicateTurn reports that a StartTurnCommand's (TeamID,
// IdempotencyKey) pair was already claimed by an earlier run. Callers
// handling platform webhook retries should treat it as successful
// delivery and drop the duplicate silently.
var ErrDuplicateTurn = errors.New("turn: duplicate idempotency key")

// ErrTeamNotServed reports that the service instance does not serve the
// command's team. The in-process runtime binds its database pool to the
// single self-hosted team, so commands for any other team must fail
// closed instead of silently reading and writing the default team's data.
var ErrTeamNotServed = errors.New("turn: team not served by this instance")

// Mode selects the turn orchestration path.
type Mode string

const (
	ModeChat    Mode = "chat"
	ModeDiscuss Mode = "discuss"
)

// Data-carrier aliases (see package comment).
type (
	Attachment            = conversation.ChatAttachment
	SkillActivation       = conversation.SkillActivation
	SkillActivationSkill  = conversation.SkillActivationSkill
	RequestedSkillContext = conversation.RequestedSkillContext
	OutboundAssetRef      = conversation.OutboundAssetRef
	InjectMessage         = conversation.InjectMessage
	ModelMessage          = conversation.ModelMessage
	AssistantOutput       = conversation.AssistantOutput
	ContentPart           = conversation.ContentPart
	ToolCall              = conversation.ToolCall
	QuestionAnswer        = userinput.QuestionAnswer
)

// UserMessageKindSkillActivation re-exports the skill-activation message kind.
const UserMessageKindSkillActivation = conversation.UserMessageKindSkillActivation

// NewSkillActivation re-exports the deduplicating constructor.
func NewSkillActivation(items []RequestedSkillContext, prompt string) *SkillActivation {
	return conversation.NewSkillActivation(items, prompt)
}

// SkillActivationModelQuery re-exports the model-query renderer.
func SkillActivationModelQuery(activation *SkillActivation) string {
	return conversation.SkillActivationModelQuery(activation)
}

// NewTextContent re-exports the plain-text content encoder.
func NewTextContent(text string) json.RawMessage {
	return conversation.NewTextContent(text)
}

// AttachmentFromBundle re-exports the bundle-to-attachment converter.
func AttachmentFromBundle(bundle attachment.Bundle) Attachment {
	return conversation.ChatAttachmentFromBundle(bundle)
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

	// DiscussMessages is the composed conversation context for a discuss
	// turn (Mode == ModeDiscuss), already rendered by the caller's
	// projection. The runtime appends its own late-binding prompt after
	// image inlining so vision parts land on the last real user message.
	DiscussMessages  []DiscussMessage
	DiscussImageRefs []DiscussImageRef
	// DiscussMentioned reports an explicit @-mention or reply-to in the
	// new context window; DiscussAddressed additionally covers direct
	// (1:1) conversations. Expensive external runtimes (ACP) use
	// DiscussAddressed as a participation gate and skip the run when
	// false.
	DiscussMentioned bool
	DiscussAddressed bool
}

// DiscussMessage is one composed context message for a discuss turn.
type DiscussMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	RawContent json.RawMessage `json:"raw_content,omitempty"`
}

// DiscussImageRef references an image attachment to inline as vision input.
type DiscussImageRef struct {
	ContentHash string `json:"content_hash"`
	Mime        string `json:"mime,omitempty"`
}

// Synthetic discuss event kinds emitted by the runtime before (or instead
// of) the model event stream.
const (
	// DiscussEventRunResolved is always the first event of a discuss run;
	// its payload is DiscussRunResolvedPayload.
	DiscussEventRunResolved = "discuss_run_resolved"
	// DiscussEventSkipped signals the runtime declined to start (e.g. ACP
	// participation gate); the run ends after this event.
	DiscussEventSkipped = "discuss_skipped"
)

// DiscussRunResolvedPayload is the payload of DiscussEventRunResolved.
type DiscussRunResolvedPayload struct {
	RuntimeType string `json:"runtime_type"`
}

// ToolApprovalResponse resumes a turn deferred on tool approval
// (RFC ResumeApprovalCommand). Mirrors flow.ToolApprovalResponseInput.
type ToolApprovalResponse struct {
	BotID                      string
	SessionID                  string
	ActorChannelIdentityID     string
	ActorUserID                string
	ApprovalID                 string
	ExplicitID                 string
	ReplyExternalMessageID     string
	Decision                   string
	Reason                     string
	ChatToken                  string
	SuppressActivePromptAttach bool
}

// UserInputResponse resumes a turn deferred on ask_user
// (RFC ResumeUserInputCommand). Mirrors flow.UserInputResponseInput.
type UserInputResponse struct {
	BotID                      string
	SessionID                  string
	ActorChannelIdentityID     string
	ActorUserID                string
	UserInputID                string
	ExplicitID                 string
	ReplyExternalMessageID     string
	Answers                    []QuestionAnswer
	TextAnswer                 string
	Canceled                   bool
	Reason                     string
	ChatToken                  string
	SuppressActivePromptAttach bool
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

// Service starts and resumes turns. Implementations: inprocess (wraps
// flow.Resolver); a cross-process transport will implement the same
// contract later. The eventCh parameters mirror the resolver's raw JSON
// stream for resumed turns; they are an in-process surface, not part of
// the serialized command shape.
type Service interface {
	StartTurn(ctx context.Context, cmd StartTurnCommand) (RunHandle, error)
	RespondToolApproval(ctx context.Context, input ToolApprovalResponse, eventCh chan<- json.RawMessage) error
	RespondUserInput(ctx context.Context, input UserInputResponse, eventCh chan<- json.RawMessage) error
	AdvancePlainTextUserInput(ctx context.Context, input userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error)
}
