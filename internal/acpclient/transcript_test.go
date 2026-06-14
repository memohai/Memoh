package acpclient

import (
	"fmt"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/event"
)

// TestTranscriptSurvivesEventBufferCap ensures transcript persistence is not
// limited by the capped UI event buffer.
func TestTranscriptSurvivesEventBufferCap(t *testing.T) {
	t.Parallel()

	collector := newEventCollector()
	total := maxCollectedStreamEvents + 100
	for i := 0; i < total; i++ {
		collector.record(event.StreamEvent{Type: event.TextDelta, Delta: fmt.Sprintf("[%d]", i)})
	}
	result := collector.result()

	if len(result.Events) != maxCollectedStreamEvents {
		t.Fatalf("UI event buffer = %d events, want capped at %d", len(result.Events), maxCollectedStreamEvents)
	}
	if len(result.Output) != 1 {
		t.Fatalf("transcript = %d messages, want 1 assistant message", len(result.Output))
	}
	text, ok := result.Output[0].Content[0].(sdk.TextPart)
	if !ok {
		t.Fatalf("transcript content = %#v, want text part", result.Output[0].Content[0])
	}
	if !strings.HasPrefix(text.Text, "[0]") {
		t.Fatalf("transcript lost its beginning: starts with %q", text.Text[:20])
	}
	if !strings.HasSuffix(text.Text, fmt.Sprintf("[%d]", total-1)) {
		t.Fatalf("transcript lost its end")
	}
}

// TestAppendTranscriptTextMergeSemantics pins the failure-note semantics:
// merge into a trailing assistant text when possible, otherwise a new
// assistant message - mirroring how the builder accumulates text while
// streaming.
func TestAppendTranscriptTextMergeSemantics(t *testing.T) {
	t.Parallel()

	merged := AppendTranscriptText([]sdk.Message{
		{Role: sdk.MessageRoleAssistant, Content: []sdk.MessagePart{sdk.TextPart{Text: "partial answer"}}},
	}, "agent failed: boom")
	if len(merged) != 1 {
		t.Fatalf("messages = %d, want merged into existing assistant", len(merged))
	}
	if text := merged[0].Content[0].(sdk.TextPart).Text; text != "partial answer\n\nagent failed: boom" {
		t.Fatalf("merged text = %q", text)
	}

	appended := AppendTranscriptText([]sdk.Message{
		{Role: sdk.MessageRoleAssistant, Content: []sdk.MessagePart{sdk.ToolCallPart{ToolCallID: "c1", ToolName: "exec"}}},
	}, "agent failed: boom")
	if len(appended) != 2 {
		t.Fatalf("messages = %d, want new assistant after tool-call message", len(appended))
	}

	fromEmpty := AppendTranscriptText(nil, "agent failed: boom")
	if len(fromEmpty) != 1 || fromEmpty[0].Role != sdk.MessageRoleAssistant {
		t.Fatalf("fromEmpty = %#v, want single assistant message", fromEmpty)
	}
}

func TestTranscriptMCPErrorResultSetsToolResultError(t *testing.T) {
	t.Parallel()

	recorder := NewTranscriptRecorder()
	recorder.Add(event.StreamEvent{
		Type:       event.ToolCallStart,
		ToolCallID: "call-1",
		ToolName:   "write",
		Input:      map[string]any{"path": "notes.txt"},
	})
	recorder.Add(event.StreamEvent{
		Type:       event.ToolCallEnd,
		ToolCallID: "call-1",
		ToolName:   "write",
		Result: map[string]any{
			"isError": true,
			"content": []map[string]any{{
				"type": "text",
				"text": "tool execution was not approved",
			}},
		},
	})

	messages := recorder.Messages("")
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want assistant tool call and tool result", messages)
	}
	result, ok := messages[1].Content[0].(sdk.ToolResultPart)
	if !ok {
		t.Fatalf("message[1] = %#v, want tool result", messages[1])
	}
	if !result.IsError {
		t.Fatalf("tool result IsError = false, want true for MCP isError result")
	}
}
