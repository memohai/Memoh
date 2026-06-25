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

func TestRenderEntryContentToolResultPreservesStructuredOutcome(t *testing.T) {
	t.Parallel()

	// A structured object with no clean text field must still reach the summarizer
	// (bounded JSON), not be dropped to a bare marker.
	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"apply_patch","toolCallId":"c1","output":{"exit_code":0,"files_changed":3}}]`),
	}
	got := renderEntryContent(mm)
	if got == "[tool result]" {
		t.Fatalf("structured outcome dropped to bare marker: %q", got)
	}
	if !strings.Contains(got, "files_changed") {
		t.Fatalf("structured outcome lost: %q", got)
	}
}

func TestRenderEntryContentToolResultTruncatesOversizedOutput(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("x", toolOutputMaxBytes*2)
	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"dump","result":{"blob":"` + big + `"}}]`),
	}
	got := renderEntryContent(mm)
	if len(got) > toolOutputMaxBytes+64 {
		t.Fatalf("oversized tool output not bounded: %d bytes", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Fatalf("expected truncation marker: %q", got[:80])
	}
}

func TestRenderEntryContentToolResultStructuredContent(t *testing.T) {
	t.Parallel()

	// Real persisted shape: SDK ToolResultPart.Result holding an arbitrary map.
	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"shell","toolCallId":"c1","result":{"structuredContent":{"stdout":"build succeeded"}}}]`),
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "build succeeded") {
		t.Fatalf("structured tool outcome lost: %q", got)
	}
	if got == "[tool result]" {
		t.Fatalf("structured tool result collapsed to bare marker")
	}
}

func TestRenderEntryContentToolResultSearchResults(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"web_search","result":{"query":"memoh","results":[{"title":"Memoh Docs","url":"https://x"}]}}]`),
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "Memoh Docs") {
		t.Fatalf("search tool outcome lost: %q", got)
	}
}

func TestRenderEntryContentToolResultMCPContentArray(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"mcp_tool","output":{"content":[{"type":"text","text":"mcp said hi"}]}}]`),
	}
	got := renderEntryContent(mm)
	if !strings.Contains(got, "mcp said hi") {
		t.Fatalf("MCP content text lost: %q", got)
	}
	if strings.Contains(got, `"type"`) {
		t.Fatalf("MCP extraction leaked structure: %q", got)
	}
}

func TestRenderEntryContentToolResultScrubsBase64Fallback(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"snapshot","result":{"image":"data:image/png;base64,QUJDREVGR0g="}}]`),
	}
	got := renderEntryContent(mm)
	if strings.Contains(got, "QUJDREVGR0g=") {
		t.Fatalf("base64 leaked into summarizer input: %q", got)
	}
}

func TestRenderEntryContentToolResultEmptyMarker(t *testing.T) {
	t.Parallel()

	mm := conversation.ModelMessage{
		Role:       "tool",
		ToolCallID: "c1",
		Content:    []byte(`[{"type":"tool-result","toolName":"noop","result":null}]`),
	}
	if got := renderEntryContent(mm); got != "[tool result]" {
		t.Fatalf("empty tool result should be a bare marker, got %q", got)
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
