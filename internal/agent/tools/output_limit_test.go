package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLimitToolOutputPrunesLargeStringLeaves(t *testing.T) {
	t.Parallel()

	large := "HEAD\n" + strings.Repeat("0123456789", 200) + "\nTAIL"
	result := LimitToolOutput(map[string]any{
		"content": large,
		"ok":      true,
	}, "test_tool", ToolOutputLimit{MaxBytes: 512, MaxLines: 80})

	structured, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want map", result)
	}
	content, ok := structured["content"].(string)
	if !ok {
		t.Fatalf("content = %#v, want string", structured["content"])
	}
	if len(content) >= len(large) {
		t.Fatalf("content was not pruned: got %d bytes, original %d", len(content), len(large))
	}
	if !strings.Contains(content, "[memoh pruned]") {
		t.Fatalf("content missing prune marker:\n%s", content)
	}
	if structured["ok"] != true {
		t.Fatalf("non-string field changed: %#v", structured)
	}
}

func TestLimitToolOutputFallsBackWhenStructuredJSONStillExceedsLimit(t *testing.T) {
	t.Parallel()

	large := map[string]any{}
	for i := 0; i < 24; i++ {
		large[fmt.Sprintf("key_%02d", i)] = strings.Repeat("x", 128)
	}

	result := LimitToolOutput(large, "huge_tool", ToolOutputLimit{MaxBytes: 512, MaxLines: 120})
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if len(raw) > 512 {
		t.Fatalf("result JSON bytes = %d, want <= 512\n%s", len(raw), raw)
	}
	structured, ok := result.(map[string]any)
	if !ok || structured["_memoh_truncated"] != true {
		t.Fatalf("result = %#v, want fallback truncation marker", result)
	}
}
