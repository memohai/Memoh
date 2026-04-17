package tools

import (
	"context"
	"testing"
)

func TestUseSkillReturnsPath(t *testing.T) {
	provider := NewSkillProvider(nil)

	toolset, err := provider.Tools(context.Background(), SessionContext{
		Skills: map[string]SkillDetail{
			"pdf": {
				Description: "Read PDF instructions",
				Content:     "Use a PDF-aware workflow.",
				Path:        "/data/.agents/skills/pdf",
			},
		},
	})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}
	if len(toolset) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolset))
	}

	result, err := toolset[0].Execute(nil, map[string]any{
		"skillName": "pdf",
		"reason":    "Need to process a PDF attachment",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	payload, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if got := payload["path"]; got != "/data/.agents/skills/pdf" {
		t.Fatalf("path = %#v, want %q", got, "/data/.agents/skills/pdf")
	}
}
