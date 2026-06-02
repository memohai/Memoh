package capabilities

import "testing"

// registryCorpus is a representative subset of real LiteLLM registry keys,
// including the bedrock/vertex/dotted prefix variants that exercise normalization.
var registryCorpus = []string{
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-opus-4-6",
	"claude-sonnet-4-6",
	"claude-sonnet-4-5",
	"claude-3-7-sonnet-20250219",
	"anthropic.claude-opus-4-8",
	"us.anthropic.claude-opus-4-8",
	"vertex_ai/claude-opus-4-8@default",
	"gpt-5",
	"gpt-5-mini",
	"o3",
	"o1",
	"gemini-2.5-pro",
	"deepseek-reasoner",
}

func TestMatch_ProviderNamingVariants(t *testing.T) {
	idx := buildIndex(registryCorpus)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"official", "claude-opus-4-8", "claude-opus-4-8"},
		{"dotted-version", "claude-opus-4.8", "claude-opus-4-8"},
		{"openrouter-prefix-reordered", "openrouter/anthropic/claude-opus-4.8", "claude-opus-4-8"},
		{"provider-prefix-dot", "anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"region-dotted-prefix", "us.anthropic.claude-opus-4-8", "claude-opus-4-8"},
		{"date-suffix", "claude-opus-4-8-20260514", "claude-opus-4-8"},
		{"bedrock-version-tag", "bedrock/us-east-1/anthropic.claude-opus-4-8-v1:0", "claude-opus-4-8"},
		{"fast-marketing-suffix", "github_copilot/claude-opus-4.8-fast", "claude-opus-4-8"},
		{"thinking-marketing-suffix", "claude-opus-4-8-thinking", "claude-opus-4-8"},
		{"reordered-tokens", "claude-4.8-opus", "claude-opus-4-8"},
		{"gpt5-plain", "openai/gpt-5", "gpt-5"},
		{"gemini-dotted", "google/gemini-2.5-pro", "gemini-2.5-pro"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := idx.match(c.input)
			if !ok {
				t.Fatalf("no match for %q", c.input)
			}
			// Compare on canonical form so equivalent keys count as a hit.
			gotCanon := normalize(got).canonical
			wantCanon := normalize(c.want).canonical
			if gotCanon != wantCanon {
				t.Fatalf("match(%q) = %q (canon %q), want canon %q", c.input, got, gotCanon, wantCanon)
			}
		})
	}
}

func TestMatch_VersionVetoPreventsCrossVersion(t *testing.T) {
	idx := buildIndex(registryCorpus)

	// 4.9 does not exist; must NOT silently fall back to 4.8/4.7/4.6.
	if got, ok := idx.match("claude-opus-4.9"); ok {
		t.Fatalf("claude-opus-4.9 should not match any 4.x key, got %q", got)
	}

	// Each existing version must resolve to its own key, never a neighbor.
	for _, in := range []struct{ input, want string }{
		{"claude-opus-4.8", "claude-opus-4-8"},
		{"claude-opus-4.7", "claude-opus-4-7"},
		{"claude-opus-4.6", "claude-opus-4-6"},
	} {
		got, ok := idx.match(in.input)
		if !ok || normalize(got).canonical != normalize(in.want).canonical {
			t.Fatalf("match(%q) = %q,%v; want %q", in.input, got, ok, in.want)
		}
	}

	// sonnet must not match opus (family token differs).
	got, ok := idx.match("claude-sonnet-4-6")
	if !ok || normalize(got).canonical != "claude-sonnet-4-6" {
		t.Fatalf("sonnet match = %q,%v", got, ok)
	}
}

func TestMatch_UnknownReturnsFalse(t *testing.T) {
	idx := buildIndex(registryCorpus)
	if got, ok := idx.match("some-totally-unknown-model-xyz"); ok {
		t.Fatalf("expected no match, got %q", got)
	}
	if _, ok := idx.match(""); ok {
		t.Fatalf("empty input should not match")
	}
}
