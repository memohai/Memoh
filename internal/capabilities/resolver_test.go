package capabilities

import "testing"

func TestCanonical(t *testing.T) {
	cases := map[string]string{
		"claude-opus-4-6":                 "claude-opus-4-6",
		"anthropic/claude-opus-4.6":       "claude-opus-4-6",
		"openrouter/anthropic/claude-4.6": "claude-4-6",
		"xai/grok-4":                      "grok-4",
		"us.anthropic.claude-opus-4-6":    "claude-opus-4-6",
		"claude-opus-4-6-20251101":        "claude-opus-4-6",
		"gpt-5.4":                         "gpt-5-4",
		"":                                "",
	}
	for in, want := range cases {
		if got := canonical(in); got != want {
			t.Errorf("canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsResellerKey(t *testing.T) {
	resellers := []string{
		"heroku/claude-3-7-sonnet", "azure_ai/gpt-oss-120b", "ovhcloud/Qwen3-32B",
		"github_copilot/gpt-5", "bedrock/anthropic.claude-opus", "openrouter/auto",
		"groq/llama-3.3-70b-versatile",
	}
	for _, k := range resellers {
		if !isResellerKey(k) {
			t.Errorf("isResellerKey(%q) = false, want true", k)
		}
	}
	firstParty := []string{
		"claude-opus-4-6", "gpt-5.4", "xai/grok-4", "mistral/mistral-large",
		"moonshot.kimi-k2-thinking", "deepseek/deepseek-reasoner",
	}
	for _, k := range firstParty {
		if isResellerKey(k) {
			t.Errorf("isResellerKey(%q) = true, want false", k)
		}
	}
}

func TestResolverPrunesResellersAndMatches(t *testing.T) {
	// Registry with an official adaptive opus, an official toggle model, and a
	// reseller shell that (if not pruned) would canonicalize identically to a
	// non-reasoning haiku and poison the match.
	body := []byte(`{
		"sample_spec": {"mode": "chat"},
		"claude-opus-4-6": {"mode": "chat", "supports_reasoning": true, "supports_adaptive_thinking": true, "supports_max_reasoning_effort": true},
		"claude-opus-4-5-20251101": {"mode": "chat", "supports_reasoning": true},
		"heroku/claude-3-7-sonnet": {"mode": "chat"},
		"claude-3-7-sonnet": {"mode": "chat", "supports_reasoning": true}
	}`)

	r, err := NewResolver(body)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	// Adaptive opus, matched via gateway-prefixed id.
	caps, ok := r.Resolve("anthropic/claude-opus-4.6")
	if !ok || caps.ThinkingMode != "adaptive" {
		t.Fatalf("opus-4.6 = %+v ok=%v, want adaptive", caps, ok)
	}
	if got := caps.EffortLevels; len(got) == 0 || got[len(got)-1] != "max" {
		t.Errorf("opus-4.6 efforts = %v, want trailing max", got)
	}

	// Toggle model with a date suffix on the input.
	caps, ok = r.Resolve("claude-opus-4-5")
	if !ok || caps.ThinkingMode != "toggle" {
		t.Errorf("opus-4.5 = %+v ok=%v, want toggle", caps, ok)
	}

	// The official claude-3-7-sonnet (reasoning) must win; the heroku shell is
	// pruned so it cannot steal the canonical slot with no reasoning.
	caps, ok = r.Resolve("claude-3-7-sonnet")
	if !ok || caps.ThinkingMode != "toggle" {
		t.Errorf("claude-3-7-sonnet = %+v ok=%v, want toggle (official, not heroku shell)", caps, ok)
	}

	// A purely reseller-served model misses (no first-party entry to borrow).
	if _, ok := r.Resolve("some-reseller-only-model"); ok {
		t.Error("unknown model unexpectedly matched")
	}
}
