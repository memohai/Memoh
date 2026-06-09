package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/capabilities"
)

func TestEnrichFileClearsStaleReasoningWhenRegistrySaysNone(t *testing.T) {
	t.Parallel()

	resolver, err := capabilities.NewResolver([]byte(`{
		"plain-model": {"mode": "chat", "supports_reasoning": false}
	}`))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "provider.yaml")
	if err := os.WriteFile(path, []byte(`name: Test
client_type: openai-completions
models:
  - model_id: plain-model
    name: Plain
    type: chat
    config:
      compatibilities: [vision, reasoning, tool-call]
      thinking_mode: toggle
      reasoning_efforts: [low, medium, high]
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	changed, err := enrichFile(path, resolver, false)
	if err != nil {
		t.Fatalf("enrichFile: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	gotBytes, err := os.ReadFile(path) //nolint:gosec // test reads its own temp fixture
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	got := string(gotBytes)
	for _, want := range []string{
		"compatibilities: [vision, tool-call]",
		"thinking_mode: none",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "reasoning_efforts") {
		t.Fatalf("reasoning_efforts should be removed:\n%s", got)
	}
}

func TestEnrichFileLeavesPlainNoReasonModelUntouched(t *testing.T) {
	t.Parallel()

	resolver, err := capabilities.NewResolver([]byte(`{
		"plain-model": {"mode": "chat", "supports_reasoning": false}
	}`))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "provider.yaml")
	raw := []byte(`name: Test
client_type: openai-completions
models:
  - model_id: plain-model
    name: Plain
    type: chat
    config:
      compatibilities: [vision, tool-call]
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	changed, err := enrichFile(path, resolver, false)
	if err != nil {
		t.Fatalf("enrichFile: %v", err)
	}
	if changed != 0 {
		t.Fatalf("changed = %d, want 0", changed)
	}
	got, err := os.ReadFile(path) //nolint:gosec // test reads its own temp fixture
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("plain no-reason model should remain untouched:\n%s", got)
	}
}
