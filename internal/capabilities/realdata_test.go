package capabilities

import (
	"context"
	"os"
	"testing"

	"github.com/memohai/memoh/internal/models"
)

// TestRealRegistry validates matching + discovery against the full LiteLLM
// registry. It is opt-in: set LITELLM_REGISTRY_JSON to a downloaded copy of
// model_prices_and_context_window.json. Without it the test skips, so CI does
// not depend on network or a vendored snapshot.
func TestRealRegistry(t *testing.T) {
	path := os.Getenv("LITELLM_REGISTRY_JSON")
	if path == "" {
		t.Skip("set LITELLM_REGISTRY_JSON to run real-registry matching validation")
	}
	body, err := os.ReadFile(path) //nolint:gosec // test reads an operator-provided fixture path
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	entries, err := parseRegistry(body)
	if err != nil {
		t.Fatalf("parse registry: %v", err)
	}
	reg := NewRegistry(withFetchFn(func(context.Context) (map[string]litellmEntry, error) {
		return entries, nil
	}), withoutBundledSnapshot())
	if err := reg.refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	t.Run("provider_naming_variants_resolve", func(t *testing.T) {
		cases := []struct {
			input            string
			wantThinkingMode string
		}{
			{"claude-opus-4-8", models.ThinkingModeAdaptive},
			{"anthropic/claude-opus-4.8", models.ThinkingModeAdaptive},
			{"openrouter/anthropic/claude-opus-4.8", models.ThinkingModeAdaptive},
			{"us.anthropic.claude-opus-4-8", models.ThinkingModeAdaptive},
			{"claude-sonnet-4-6", models.ThinkingModeAdaptive},
			{"openai/gpt-5", models.ThinkingModeToggle},
			{"o3", models.ThinkingModeToggle},
			{"google/gemini-2.5-pro", models.ThinkingModeToggle},
		}
		for _, c := range cases {
			caps, ok := reg.Lookup(context.Background(), c.input)
			if !ok {
				t.Errorf("%q: no match", c.input)
				continue
			}
			if caps.ThinkingMode != c.wantThinkingMode {
				t.Errorf("%q: thinking mode = %q, want %q", c.input, caps.ThinkingMode, c.wantThinkingMode)
			}
		}
	})

	t.Run("no_cross_version_match", func(t *testing.T) {
		// A non-existent version must not borrow a neighbor's capabilities.
		for _, in := range []string{"claude-opus-4-99", "gpt-5000", "o99"} {
			if caps, ok := reg.Lookup(context.Background(), in); ok {
				t.Errorf("%q unexpectedly matched (thinking=%q)", in, caps.ThinkingMode)
			}
		}
	})

	t.Run("every_adaptive_key_round_trips", func(t *testing.T) {
		for key, e := range entries {
			if e.SupportsAdaptiveThinking == nil || !*e.SupportsAdaptiveThinking {
				continue
			}
			caps, ok := reg.Lookup(context.Background(), key)
			if !ok {
				t.Errorf("adaptive key %q did not round-trip match", key)
				continue
			}
			if caps.ThinkingMode != models.ThinkingModeAdaptive {
				t.Errorf("adaptive key %q resolved to %q", key, caps.ThinkingMode)
			}
		}
	})
}
