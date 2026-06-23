package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLimitToolResultCapsFullMCPResult(t *testing.T) {
	t.Parallel()

	result := LimitToolResult(BuildToolSuccessResult(map[string]any{
		"content": "HEAD\n" + strings.Repeat("0123456789", 200) + "\nTAIL",
		"ok":      true,
	}), "test_tool", ToolOutputLimit{MaxBytes: 512, MaxLines: 80})

	assertJSONBytesAtMost(t, result, 512)
	text := toolResultText(result)
	if !strings.Contains(text, "[memoh pruned]") {
		t.Fatalf("limited MCP result missing prune marker: %#v", result)
	}
}

func TestLimitToolResultPreservesErrorSignal(t *testing.T) {
	t.Parallel()

	result := LimitToolResult(BuildToolErrorResult("HEAD\n"+strings.Repeat("error detail ", 300)+"\nTAIL"), "error_tool", ToolOutputLimit{MaxBytes: 512, MaxLines: 80})

	assertJSONBytesAtMost(t, result, 512)
	if isError, _ := result["isError"].(bool); !isError {
		t.Fatalf("limited MCP error result lost isError: %#v", result)
	}
	text := toolResultText(result)
	if !strings.Contains(text, "[memoh pruned]") {
		t.Fatalf("limited MCP error missing prune marker: %#v", result)
	}
}

func assertJSONBytesAtMost(t *testing.T, value any, max int) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	if len(raw) > max {
		t.Fatalf("JSON bytes = %d, want <= %d\n%s", len(raw), max, raw)
	}
}
