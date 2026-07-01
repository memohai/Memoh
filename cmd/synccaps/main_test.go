package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

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
	cfg := readOnlyModelConfig(t, path)
	if got := cfg.ThinkingMode; got != "none" {
		t.Fatalf("thinking_mode = %q, want none", got)
	}
	if len(cfg.ReasoningEfforts) != 0 {
		t.Fatalf("reasoning_efforts = %v, want empty", cfg.ReasoningEfforts)
	}
	for _, want := range []string{"vision", "tool-call"} {
		if !slices.Contains(cfg.Compatibilities, want) {
			t.Fatalf("compatibilities missing %q", want)
		}
	}
	if slices.Contains(cfg.Compatibilities, "reasoning") {
		t.Fatal("reasoning compatibility should be removed")
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
}

func TestEnrichFileAddsReasoningMetadata(t *testing.T) {
	t.Parallel()

	resolver, err := capabilities.NewResolver([]byte(`{
		"reasoning-model": {"mode": "chat", "supports_reasoning": true, "supports_max_reasoning_effort": true}
	}`))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "provider.yaml")
	if err := os.WriteFile(path, []byte(`name: Test
client_type: openai-completions
models:
  - model_id: reasoning-model
    name: Reasoning
    type: chat
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
	cfg := readOnlyModelConfig(t, path)
	if got := cfg.ThinkingMode; got != "toggle" {
		t.Fatalf("thinking_mode = %q, want toggle", got)
	}
	if !slices.Equal(cfg.ReasoningEfforts, []string{"low", "medium", "high", "max"}) {
		t.Fatalf("reasoning_efforts = %v, want [low medium high max]", cfg.ReasoningEfforts)
	}
}

type providerFixture struct {
	Models []struct {
		Config modelConfig `yaml:"config"`
	} `yaml:"models"`
}

type modelConfig struct {
	ThinkingMode     string   `yaml:"thinking_mode"`
	ReasoningEfforts []string `yaml:"reasoning_efforts"`
	Compatibilities  []string `yaml:"compatibilities"`
}

func readOnlyModelConfig(t *testing.T, path string) modelConfig {
	t.Helper()

	raw, err := os.ReadFile(path) //nolint:gosec // test reads its own temp fixture
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var doc providerFixture
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if len(doc.Models) != 1 {
		t.Fatalf("models length = %d, want 1", len(doc.Models))
	}
	return doc.Models[0].Config
}
