package handlers

import (
	"encoding/json"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestFormatLocalStreamEvent_UsesChannelEventShape(t *testing.T) {
	t.Parallel()

	data, err := formatLocalStreamEvent(channel.StreamEvent{
		Type:  channel.StreamEventDelta,
		Delta: "hello",
		Phase: channel.StreamPhaseText,
	})
	if err != nil {
		t.Fatalf("format local stream event failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if got := payload["type"]; got != "delta" {
		t.Fatalf("expected type delta, got %#v", got)
	}
	if got := payload["delta"]; got != "hello" {
		t.Fatalf("expected delta hello, got %#v", got)
	}
	if got := payload["phase"]; got != "text" {
		t.Fatalf("expected phase text, got %#v", got)
	}
	if _, ok := payload["target"]; ok {
		t.Fatalf("unexpected wrapper field target in payload")
	}
	if _, ok := payload["event"]; ok {
		t.Fatalf("unexpected wrapper field event in payload")
	}
}

func TestFormatLocalStreamEvent_EncodesToolCallAsToolCallObject(t *testing.T) {
	t.Parallel()

	data, err := formatLocalStreamEvent(channel.StreamEvent{
		Type: channel.StreamEventToolCallStart,
		ToolCall: &channel.StreamToolCall{
			Name:   "exec",
			CallID: "call-1",
			Input: map[string]any{
				"command": "pwd",
			},
		},
	})
	if err != nil {
		t.Fatalf("format local stream event failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	toolCall, ok := payload["tool_call"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_call object, got %#v", payload["tool_call"])
	}
	if got := toolCall["name"]; got != "exec" {
		t.Fatalf("expected tool_call.name exec, got %#v", got)
	}
	if got := toolCall["call_id"]; got != "call-1" {
		t.Fatalf("expected tool_call.call_id call-1, got %#v", got)
	}
	if _, ok := payload["toolName"]; ok {
		t.Fatalf("unexpected camelCase toolName in payload")
	}
}
