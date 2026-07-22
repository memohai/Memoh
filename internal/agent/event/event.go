// Package event defines the stream events shared by agent runtimes and the
// conversation layer.
package event

import "encoding/json"

// StreamEventType identifies the kind of stream event.
type StreamEventType string

const (
	AgentStart          StreamEventType = "agent_start"
	HistoryCommit       StreamEventType = "history_commit"
	TextStart           StreamEventType = "text_start"
	TextDelta           StreamEventType = "text_delta"
	TextEnd             StreamEventType = "text_end"
	ReasoningStart      StreamEventType = "reasoning_start"
	ReasoningDelta      StreamEventType = "reasoning_delta"
	ReasoningEnd        StreamEventType = "reasoning_end"
	ToolCallInputStart  StreamEventType = "tool_call_input_start"
	ToolCallStart       StreamEventType = "tool_call_start"
	ToolCallMetadata    StreamEventType = "tool_call_metadata"
	ToolCallProgress    StreamEventType = "tool_call_progress"
	ToolCallEnd         StreamEventType = "tool_call_end"
	ToolApprovalRequest StreamEventType = "tool_approval_request"
	UserInputRequest    StreamEventType = "user_input_request"
	Attachment          StreamEventType = "attachment_delta"
	Reaction            StreamEventType = "reaction_delta"
	Speech              StreamEventType = "speech_delta"
	AgentEnd            StreamEventType = "agent_end"
	AgentAbort          StreamEventType = "agent_abort"
	Retry               StreamEventType = "retry"
	Progress            StreamEventType = "progress"
	Error               StreamEventType = "error"
	ModelStepStart      StreamEventType = "model_step_start"
)

// RowIdentity identifies one durable history row represented by a projected
// stream block. A tool block may reference both call and result rows.
type RowIdentity struct {
	StableID       string `json:"stable_id"`
	Role           string `json:"role,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	TurnPosition   int64  `json:"turn_position"`
	TurnMessageSeq int64  `json:"turn_message_seq"`
}

// StreamEvent is emitted by an agent runtime during streaming. The JSON
// shape is the wire format WebSocket clients consume; do not change tags.
type StreamEvent struct {
	Type             StreamEventType  `json:"type"`
	Delta            string           `json:"delta,omitempty"`
	ToolName         string           `json:"toolName,omitempty"`
	ToolCallID       string           `json:"toolCallId,omitempty"`
	ApprovalID       string           `json:"approvalId,omitempty"`
	UserInputID      string           `json:"userInputId,omitempty"`
	ShortID          int              `json:"shortId,omitempty"`
	Status           string           `json:"status,omitempty"`
	Input            any              `json:"input,omitempty"`
	Metadata         map[string]any   `json:"metadata,omitempty"`
	Progress         any              `json:"progress,omitempty"`
	Result           any              `json:"result,omitempty"`
	Attachments      []FileAttachment `json:"attachments,omitempty"`
	Reactions        []ReactionItem   `json:"reactions,omitempty"`
	Speeches         []SpeechItem     `json:"speeches,omitempty"`
	Messages         json.RawMessage  `json:"messages,omitempty"`
	Usage            json.RawMessage  `json:"usage,omitempty"`
	HistoryCommitted bool             `json:"history_committed,omitempty"`
	Reasoning        []string         `json:"reasoning,omitempty"`
	Error            string           `json:"error,omitempty"`
	Attempt          int              `json:"attempt,omitempty"`
	MaxAttempt       int              `json:"maxAttempt,omitempty"`
	RetryError       string           `json:"retryError,omitempty"`
	StepNumber       int              `json:"stepNumber,omitempty"`
	TotalSteps       int              `json:"totalSteps,omitempty"`
	ProgressStatus   string           `json:"progressStatus,omitempty"`
	StableID         string           `json:"stable_id,omitempty"`
	TurnID           string           `json:"turn_id,omitempty"`
	TurnPosition     int64            `json:"turn_position,omitempty"`
	TurnMessageSeq   int64            `json:"turn_message_seq,omitempty"`
	RowIdentities    []RowIdentity    `json:"row_identities,omitempty"`
	LedgerRows       []RowIdentity    `json:"ledger_rows,omitempty"`
	ResetLedger      bool             `json:"reset_ledger,omitempty"`
}

// IsTerminal returns true for events that signal end of stream.
func (e StreamEvent) IsTerminal() bool {
	return e.Type == AgentEnd || e.Type == AgentAbort
}

// FileAttachment represents a file reference extracted from agent output.
type FileAttachment struct {
	Type        string         `json:"type"`
	Base64      string         `json:"base64,omitempty"`
	Path        string         `json:"path,omitempty"`
	URL         string         `json:"url,omitempty"`
	PlatformKey string         `json:"platform_key,omitempty"`
	Mime        string         `json:"mime,omitempty"`
	Name        string         `json:"name,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	Size        int64          `json:"size,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ReactionItem represents an emoji reaction extracted from agent output.
type ReactionItem struct {
	Emoji string `json:"emoji"`
}

// SpeechItem represents a TTS request extracted from agent output.
type SpeechItem struct {
	Text string `json:"text"`
}
