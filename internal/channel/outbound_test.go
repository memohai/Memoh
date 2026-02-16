package channel

import (
	"testing"
)

type streamValidationAdapter struct {
	channelType ChannelType
}

func (a *streamValidationAdapter) Type() ChannelType {
	return a.channelType
}

func (a *streamValidationAdapter) Descriptor() Descriptor {
	return Descriptor{
		Type:        a.channelType,
		DisplayName: "stream-validation",
		Capabilities: ChannelCapabilities{
			Text:           true,
			Attachments:    true,
			Streaming:      true,
			BlockStreaming: true,
		},
	}
}

func newStreamValidationRegistry(t *testing.T) *Registry {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(&streamValidationAdapter{channelType: ChannelType("test")}); err != nil {
		t.Fatalf("register adapter failed: %v", err)
	}
	return registry
}

func TestValidateStreamEventSupportedTypes(t *testing.T) {
	t.Parallel()

	registry := newStreamValidationRegistry(t)
	channelType := ChannelType("test")
	tests := []struct {
		name  string
		event StreamEvent
	}{
		{name: "status", event: StreamEvent{Type: StreamEventStatus, Status: StreamStatusStarted}},
		{name: "delta", event: StreamEvent{Type: StreamEventDelta, Delta: "hello"}},
		{name: "phase start", event: StreamEvent{Type: StreamEventPhaseStart, Phase: StreamPhaseText}},
		{name: "phase end", event: StreamEvent{Type: StreamEventPhaseEnd, Phase: StreamPhaseText}},
		{name: "tool start", event: StreamEvent{Type: StreamEventToolCallStart, ToolCall: &StreamToolCall{Name: "search"}}},
		{name: "tool end", event: StreamEvent{Type: StreamEventToolCallEnd, ToolCall: &StreamToolCall{Name: "search"}}},
		{name: "attachment", event: StreamEvent{Type: StreamEventAttachment, Attachments: []Attachment{{Type: AttachmentImage, URL: "https://example.com/img.png"}}}},
		{name: "agent start", event: StreamEvent{Type: StreamEventAgentStart}},
		{name: "agent end", event: StreamEvent{Type: StreamEventAgentEnd}},
		{name: "processing started", event: StreamEvent{Type: StreamEventProcessingStarted}},
		{name: "processing completed", event: StreamEvent{Type: StreamEventProcessingCompleted}},
		{name: "processing failed", event: StreamEvent{Type: StreamEventProcessingFailed, Error: "failed"}},
		{name: "final", event: StreamEvent{Type: StreamEventFinal, Final: &StreamFinalizePayload{Message: Message{Text: "done"}}}},
		{name: "error", event: StreamEvent{Type: StreamEventError, Error: "boom"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateStreamEvent(registry, channelType, tt.event); err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestValidateStreamEventInvalidPayload(t *testing.T) {
	t.Parallel()

	registry := newStreamValidationRegistry(t)
	channelType := ChannelType("test")
	tests := []struct {
		name  string
		event StreamEvent
	}{
		{name: "missing status", event: StreamEvent{Type: StreamEventStatus}},
		{name: "missing tool call payload", event: StreamEvent{Type: StreamEventToolCallStart}},
		{name: "empty attachment payload", event: StreamEvent{Type: StreamEventAttachment}},
		{name: "processing failed missing error", event: StreamEvent{Type: StreamEventProcessingFailed}},
		{name: "missing final payload", event: StreamEvent{Type: StreamEventFinal}},
		{name: "missing error payload", event: StreamEvent{Type: StreamEventError}},
		{name: "unsupported type", event: StreamEvent{Type: StreamEventType("unknown")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateStreamEvent(registry, channelType, tt.event); err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}
}
