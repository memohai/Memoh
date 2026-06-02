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
	"github_copilot/claude-opus-4-6-fast",
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
		{"fast-is-a-distinct-variant", "anthropic/claude-opus-4.6-fast", "claude-opus-4-6-fast"},
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

// TestMatch_VariantDoesNotCollapseToBase guards the highest-impact failure
// mode: when the registry carries a base model but NOT a sibling variant, the
// variant must MISS (fall back to safe defaults) rather than borrow the base's
// capabilities/context window. e.g. deepseek-v4-pro and deepseek-v4-flash have
// different context windows, so neither may resolve to a bare deepseek-v4.
func TestMatch_VariantDoesNotCollapseToBase(t *testing.T) {
	idx := buildIndex([]string{
		"deepseek-v4",
		"deepseek-v3.2",
		"gpt-5",
		"qwen3-coder",
		"gemini-3-flash",  // base flash present, "lite" variant absent
		"claude-opus-4-8", // base present, "-fast" variant absent
	})

	for _, variant := range []string{
		"deepseek/deepseek-v4-pro",
		"deepseek/deepseek-v4-flash",
		"openai/gpt-5-pro",
		"openai/gpt-5-mini",
		"openai/gpt-5-nano",
		"qwen/qwen3-coder-plus",
		"google/gemini-3-flash-lite",
		// fast / exp are capability-distinguishing now; without their own key
		// they must MISS, not borrow the base model.
		"anthropic/claude-opus-4.8-fast",
		"deepseek/deepseek-v3.2-exp",
	} {
		if got, ok := idx.match(variant); ok {
			t.Fatalf("variant %q must not collapse to a base model, got %q", variant, got)
		}
	}

	// Sanity: the bare base names still resolve exactly.
	for _, base := range []string{"deepseek-v4", "gpt-5", "qwen3-coder"} {
		if _, ok := idx.match(base); !ok {
			t.Fatalf("base %q should still match", base)
		}
	}
}

// TestNormalize_BareVersionSuffixSurvives guards against the bedrock-revision
// regex eating a legitimate bare "-vN" version token. Distinct bare versions
// (titan-...-v1 vs -v2, deepseek-v4 vs -v2) must keep their version signature
// and never collapse to one canonical / cross-match.
func TestNormalize_BareVersionSuffixSurvives(t *testing.T) {
	for _, c := range []struct {
		raw      string
		wantVer  string
		wantCanT bool // version token expected present
	}{
		{"deepseek-v4", "v4", true},
		{"amazon.titan-image-generator-v2", "v2", true},
		{"anthropic.claude-v1", "v1", true},
	} {
		n := normalize(c.raw)
		if _, ok := n.versions[c.wantVer]; ok != c.wantCanT {
			t.Fatalf("normalize(%q).versions = %v, want %q present=%v", c.raw, n.versions, c.wantVer, c.wantCanT)
		}
	}

	// Bedrock revision (with colon) is still stripped.
	if _, ok := normalize("anthropic.claude-opus-4-8-v1:0").versions["v1"]; ok {
		t.Fatalf("bedrock -v1:0 revision should be stripped, not kept as version")
	}

	// Distinct bare versions must not cross-match.
	idx := buildIndex([]string{
		"amazon.titan-image-generator-v1",
		"amazon.titan-image-generator-v2",
		"deepseek-v4",
		"deepseek-v2",
	})
	got1, _ := idx.match("amazon.titan-image-generator-v1")
	got2, _ := idx.match("amazon.titan-image-generator-v2")
	if normalize(got1).canonical == normalize(got2).canonical {
		t.Fatalf("titan v1 and v2 collapsed to one canonical: %q vs %q", got1, got2)
	}
	if got, ok := idx.match("deepseek-v3"); ok {
		t.Fatalf("deepseek-v3 (absent) must not match a neighbor version, got %q", got)
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
