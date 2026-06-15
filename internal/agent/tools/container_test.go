package tools

import (
	"context"
	"strings"
	"testing"
)

const readImageHint = "Also supports reading image files (PNG, JPEG, GIF, WebP)"

func readToolDescription(t *testing.T, supportsImageInput bool) string {
	t.Helper()
	provider := NewContainerProvider(nil, nil, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: supportsImageInput,
	})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	for _, tool := range toolList {
		if tool.Name == "read" {
			return tool.Description
		}
	}
	t.Fatalf("read tool not found in container provider tools")
	return ""
}

func TestContainerReadDescriptionIncludesImageHintWhenImageInputSupported(t *testing.T) {
	t.Parallel()
	desc := readToolDescription(t, true)
	if !strings.Contains(desc, readImageHint) {
		t.Fatalf("expected read tool description to contain %q, got:\n%s", readImageHint, desc)
	}
}

func TestContainerReadDescriptionOmitsImageHintWhenImageInputUnsupported(t *testing.T) {
	t.Parallel()
	desc := readToolDescription(t, false)
	if strings.Contains(desc, readImageHint) {
		t.Fatalf("expected read tool description to NOT contain %q, got:\n%s", readImageHint, desc)
	}
}

func TestContainerApplyPatchDescriptionDoesNotReferenceSiblingTools(t *testing.T) {
	t.Parallel()

	provider := NewContainerProvider(nil, nil, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	for _, tool := range toolList {
		if tool.Name != ToolApplyPatch.String() {
			continue
		}
		for _, absent := range []string{"Use edit", "Use write", "`edit`", "`write`"} {
			if strings.Contains(tool.Description, absent) {
				t.Fatalf("apply_patch description references sibling tool %q:\n%s", absent, tool.Description)
			}
		}
		return
	}
	t.Fatalf("apply_patch tool not found")
}

func TestDetectBlockedSleep(t *testing.T) {
	tests := []struct {
		command string
		blocked bool
	}{
		// Should block
		{"sleep 5", true},
		{"sleep 10", true},
		{"sleep 30", true},
		{"sleep 5 && echo done", true},
		{"sleep 5; echo done", true},

		// Should allow
		{"sleep 1", false},       // under 2 seconds
		{"sleep 0.5", false},     // under 2 seconds
		{"echo hello", false},    // not sleep
		{"npm install", false},   // not sleep
		{"echo sleep 5", false},  // sleep not at start
		{"cat sleep.txt", false}, // not the sleep command
	}

	for _, tt := range tests {
		result := detectBlockedSleep(tt.command)
		if tt.blocked && result == "" {
			t.Errorf("expected %q to be blocked, but it was allowed", tt.command)
		}
		if !tt.blocked && result != "" {
			t.Errorf("expected %q to be allowed, but got: %s", tt.command, result)
		}
	}
}
