package capabilities

import "testing"

func TestCanonical(t *testing.T) {
	cases := map[string]string{
		"openrouter/anthropic/claude-4.6": "claude-4-6",
		"us.anthropic.claude-opus-4-6":    "claude-opus-4-6",
		"claude-opus-4-6-20251101":        "claude-opus-4-6",
		"gpt-5.4":                         "gpt-5-4",
		"deepseek-coder":                  "deepseek-coder",
		"dashscope.qwen-coder":            "qwen-coder",
	}
	for in, want := range cases {
		if got := canonical(in); got != want {
			t.Errorf("canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolverDoesNotCollapseModelFamilies(t *testing.T) {
	body := []byte(`{
		"deepseek-coder": {"mode": "chat", "supports_reasoning": false},
		"dashscope/qwen-coder": {"mode": "chat", "supports_reasoning": true, "supports_max_reasoning_effort": true}
	}`)
	r, err := NewResolver(body)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	deepseek, ok := r.Resolve("deepseek/deepseek-coder")
	if !ok {
		t.Fatal("deepseek-coder should resolve")
	}
	if deepseek.ThinkingMode != "none" {
		t.Fatalf("deepseek-coder thinking = %q, want none; likely folded into qwen-coder", deepseek.ThinkingMode)
	}

	qwen, ok := r.Resolve("dashscope/qwen-coder")
	if !ok || qwen.ThinkingMode != "toggle" {
		t.Fatalf("qwen-coder = %+v ok=%v, want toggle", qwen, ok)
	}
}

func TestResolverDoesNotBorrowMissingVariants(t *testing.T) {
	body := []byte(`{
		"deepseek-v4": {"mode": "chat", "supports_reasoning": true},
		"gpt-5": {"mode": "chat", "supports_reasoning": true},
		"qwen3-coder": {"mode": "chat", "supports_reasoning": true},
		"kimi-k2-thinking": {"mode": "chat", "supports_reasoning": true}
	}`)
	r, err := NewResolver(body)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	for _, modelID := range []string{
		"deepseek/deepseek-v4-pro",
		"openai/gpt-5-mini",
		"qwen/qwen3-coder-plus",
		"kimi-k2",
	} {
		if caps, ok := r.Resolve(modelID); ok {
			t.Fatalf("%q unexpectedly borrowed capabilities: %+v", modelID, caps)
		}
	}
}

func TestResolverPrunesResellersAndMatches(t *testing.T) {
	// Registry with official matches plus reseller-only shells. The shells must
	// be pruned so they cannot provide capabilities without a first-party entry.
	body := []byte(`{
		"sample_spec": {"mode": "chat"},
		"claude-opus-4-6": {"mode": "chat", "supports_reasoning": true, "supports_adaptive_thinking": true, "supports_max_reasoning_effort": true},
		"claude-opus-4-5-20251101": {"mode": "chat", "supports_reasoning": true},
		"bedrock.reseller-only-model": {"mode": "chat", "supports_reasoning": true},
		"openrouter/reseller-only-model": {"mode": "chat", "supports_reasoning": true},
		"xai/grok-4": {"mode": "chat", "supports_reasoning": true}
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

	// Vendor-owned first-party prefixes are retained, not treated as resellers.
	caps, ok = r.Resolve("xai/grok-4")
	if !ok || caps.ThinkingMode != "toggle" {
		t.Errorf("grok-4 = %+v ok=%v, want toggle", caps, ok)
	}

	if _, ok := r.Resolve("reseller-only-model"); ok {
		t.Error("reseller-only model unexpectedly matched")
	}
}
