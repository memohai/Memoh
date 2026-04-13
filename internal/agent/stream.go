package agent

import "encoding/json"

// StreamEventType identifies the kind of stream event.
type StreamEventType string

const (
	EventAgentStart       StreamEventType = "agent_start"
	EventTextStart        StreamEventType = "text_start"
	EventTextDelta        StreamEventType = "text_delta"
	EventTextEnd          StreamEventType = "text_end"
	EventReasoningStart   StreamEventType = "reasoning_start"
	EventReasoningDelta   StreamEventType = "reasoning_delta"
	EventReasoningEnd     StreamEventType = "reasoning_end"
	EventToolCallStart    StreamEventType = "tool_call_start"
	EventToolCallProgress StreamEventType = "tool_call_progress"
	EventToolCallEnd      StreamEventType = "tool_call_end"
	EventAttachment       StreamEventType = "attachment_delta"
	EventReaction         StreamEventType = "reaction_delta"
	EventSpeech           StreamEventType = "speech_delta"
	EventAgentEnd         StreamEventType = "agent_end"
	EventAgentAbort       StreamEventType = "agent_abort"
	EventRetry            StreamEventType = "retry"
	EventProgress         StreamEventType = "progress"
	EventError            StreamEventType = "error"
)

// StreamEvent is emitted by the agent during streaming.
type StreamEvent struct {
	Type           StreamEventType  `json:"type"`
	Delta          string           `json:"delta,omitempty"`
	ToolName       string           `json:"toolName,omitempty"`
	ToolCallID     string           `json:"toolCallId,omitempty"`
	Input          any              `json:"input,omitempty"`
	Progress       any              `json:"progress,omitempty"`
	Result         any              `json:"result,omitempty"`
	Attachments    []FileAttachment `json:"attachments,omitempty"`
	Reactions      []ReactionItem   `json:"reactions,omitempty"`
	Speeches       []SpeechItem     `json:"speeches,omitempty"`
	Messages       json.RawMessage  `json:"messages,omitempty"`
	Usage          json.RawMessage  `json:"usage,omitempty"`
	Reasoning      []string         `json:"reasoning,omitempty"`
	Error          string           `json:"error,omitempty"`
	Attempt        int              `json:"attempt,omitempty"`
	MaxAttempt     int              `json:"maxAttempt,omitempty"`
	RetryError     string           `json:"retryError,omitempty"`
	StepNumber     int              `json:"stepNumber,omitempty"`
	TotalSteps     int              `json:"totalSteps,omitempty"`
	ProgressStatus string           `json:"progressStatus,omitempty"`
}

// IsTerminal returns true for events that signal end of stream.
func (e StreamEvent) IsTerminal() bool {
	return e.Type == EventAgentEnd || e.Type == EventAgentAbort
}
