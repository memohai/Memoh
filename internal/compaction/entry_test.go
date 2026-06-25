package compaction

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestRenderEntryContentPlainString(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("hello world")}
	if got := renderEntryContent(mm); got != "hello world" {
		t.Fatalf("plain string render = %q, want 'hello world'", got)
	}
}

func TestRenderEntryContentExtractsTextFromParts(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:    "user",
		Content: []byte(`[{"type":"text","text":"clean text"}]`),
	}
	got := renderEntryContent(mm)
	if got != "clean text" {
		t.Fatalf("render = %q, want 'clean text' (no JSON envelope noise)", got)
	}
	if strings.Contains(got, `"type"`) || strings.Contains(got, "{") {
		t.Fatalf("render leaked JSON structure: %q", got)
	}
}

func TestRenderEntryContentStripsImageBase64(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:    "user",
		Content: []byte(`[{"type":"text","text":"look at this"},{"type":"image","image":"data:image/png;base64,QUJDRA==","mediaType":"image/png"}]`),
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "look at this") {
		t.Fatalf("render dropped text: %q", got)
	}
	if !strings.Contains(got, "[image]") {
		t.Fatalf("render should mark image presence: %q", got)
	}
	if strings.Contains(got, "base64") || strings.Contains(got, "QUJDRA==") {
		t.Fatalf("render leaked base64 image payload: %q", got)
	}
}

func TestRenderEntryContentToolCallName(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:    "assistant",
		Content: []byte(`[{"type":"tool-call","toolName":"read_file","toolCallId":"c1","input":{"path":"/a"}}]`),
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "read_file") {
		t.Fatalf("render should mention tool call name: %q", got)
	}
}

func TestRenderEntryContentToolResultOutcomeNotRawJSON(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"read_file","toolCallId":"c1","output":{"type":"text","value":"the file said hi"}}]`),
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "the file said hi") {
		t.Fatalf("render should surface tool outcome: %q", got)
	}
	if strings.Contains(got, "toolCallId") || strings.Contains(got, `"output"`) {
		t.Fatalf("render leaked raw tool-result JSON: %q", got)
	}
}

func TestRenderEntryContentToolResultFallbackMarker(t *testing.T) {
	t.Parallel()

	// Output is a non-text object (e.g. structured/binary) with no extractable string.
	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"snapshot","toolCallId":"c1","output":{"type":"image","data":"QUJDRA=="}}]`),
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "[tool result]") {
		t.Fatalf("render should fall back to a tool-result marker: %q", got)
	}
	if strings.Contains(got, "QUJDRA==") {
		t.Fatalf("render leaked binary tool output: %q", got)
	}
}

func TestRenderEntryContentTopLevelToolCalls(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role: "assistant",
		ToolCalls: []conversation.ToolCall{
			{ID: "c1", Type: "function", Function: conversation.ToolCallFunction{Name: "search", Arguments: `{"q":"x"}`}},
		},
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "search") {
		t.Fatalf("render should mention top-level tool call: %q", got)
	}
}
