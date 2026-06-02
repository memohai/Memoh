package capabilities

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestMatchMatrix is an opt-in diagnostic that runs the matcher over a curated
// set of real provider/gateway model identifiers against the live LiteLLM
// registry, printing a matrix of input -> matched key -> context window /
// thinking mode, and flagging two failure classes:
//
//   - COLLISION: two distinct inputs resolve to the same registry key.
//   - VARIANT-COLLAPSE: an input carrying a size/variant token (flash, pro,
//     mini, nano, lite, fast, air) matches a key that lacks that token, i.e.
//     it borrowed a sibling/base model's capabilities.
//
// Run with: LITELLM_REGISTRY_JSON=/tmp/litellm_registry.json go test \
//
//	./internal/capabilities -run TestMatchMatrix -v
func TestMatchMatrix(t *testing.T) {
	path := os.Getenv("LITELLM_REGISTRY_JSON")
	if path == "" {
		t.Skip("set LITELLM_REGISTRY_JSON to run the match matrix")
	}
	body, err := os.ReadFile(path) //nolint:gosec // diagnostic reads an operator-provided fixture path
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	entries, err := parseRegistry(body)
	if err != nil {
		t.Fatalf("parse registry: %v", err)
	}
	idx := buildIndex(keysOf(entries))

	// Real OpenRouter ids (provider/slug) plus a few canonical/gateway variants,
	// grouped by family. These are the actual names a provider import would feed
	// the matcher.
	inputs := []string{
		// DeepSeek — the user's primary concern: V4 flash vs pro have different
		// context windows and capability shapes.
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4-pro",
		"deepseek/deepseek-chat-v3.1",
		"deepseek/deepseek-v3.2",
		"deepseek/deepseek-v3.2-exp",
		"deepseek/deepseek-r1",
		"deepseek-chat",
		"deepseek-reasoner",
		// OpenAI — base vs pro/mini/nano/codex must not collapse.
		"openai/gpt-5",
		"openai/gpt-5-pro",
		"openai/gpt-5-mini",
		"openai/gpt-5-nano",
		"openai/gpt-5-codex",
		"openai/gpt-5.1",
		"openai/gpt-5.2-pro",
		"openai/gpt-5.4-mini",
		"openai/o4-mini",
		"openai/o4-mini-high",
		// Anthropic — fast variants + version discrimination.
		"anthropic/claude-opus-4.8",
		"anthropic/claude-opus-4.8-fast",
		"anthropic/claude-opus-4.6",
		"anthropic/claude-opus-4.6-fast",
		"anthropic/claude-sonnet-4.6",
		"anthropic/claude-haiku-4.5",
		"us.anthropic.claude-opus-4-8-v1:0",
		"claude-opus-4-8",
		// Xiaomi MiMo.
		"xiaomi/mimo-v2-flash",
		"xiaomi/mimo-v2.5",
		"xiaomi/mimo-v2.5-pro",
		// Google Gemini.
		"google/gemini-3-flash-preview",
		"google/gemini-3.1-flash-lite",
		"google/gemini-3.1-pro-preview",
		"google/gemini-3.5-flash",
		// Qwen.
		"qwen/qwen3-max",
		"qwen/qwen3-coder-flash",
		"qwen/qwen3-coder-plus",
		// xAI.
		"x-ai/grok-4.3",
	}

	variantTokens := map[string]bool{
		"flash": true, "pro": true, "mini": true, "nano": true,
		"lite": true, "fast": true, "air": true, "max": true, "chat": true,
	}

	type row struct {
		input, key string
		ctx        string
		mode       string
		collapse   bool
		miss       bool
	}

	keyToInputs := map[string][]string{}
	rows := make([]row, 0, len(inputs))

	for _, in := range inputs {
		key, ok := idx.match(in)
		r := row{input: in, key: key}
		if !ok {
			r.miss = true
			rows = append(rows, r)
			continue
		}
		keyToInputs[key] = append(keyToInputs[key], in)

		e := entries[key]
		if e.MaxInputTokens != nil {
			r.ctx = strconv.Itoa(*e.MaxInputTokens)
		} else {
			r.ctx = "-"
		}
		caps := derive(e)
		r.mode = caps.ThinkingMode
		if r.mode == "" {
			r.mode = "?"
		}

		// Variant-collapse: the input has a variant token the matched key lacks.
		inNorm := normalize(in)
		keyNorm := normalize(key)
		for tok := range inNorm.tokens {
			if variantTokens[tok] {
				if _, present := keyNorm.tokens[tok]; !present {
					r.collapse = true
				}
			}
		}
		rows = append(rows, r)
	}

	// Print the matrix.
	var b strings.Builder
	fmt.Fprintf(&b, "\n=== MATCH MATRIX (registry models: %d) ===\n", len(entries))
	fmt.Fprintf(&b, "%-38s %-34s %-10s %-14s %s\n", "INPUT", "MATCHED KEY", "CTX", "THINKING", "FLAG")
	b.WriteString(strings.Repeat("-", 110) + "\n")
	matched, missed, collapsed := 0, 0, 0
	for _, r := range rows {
		flag := ""
		switch {
		case r.miss:
			flag = "MISS (no fill — safe)"
			missed++
		case r.collapse:
			flag = "** VARIANT-COLLAPSE **"
			collapsed++
			matched++
		default:
			matched++
		}
		key := r.key
		if r.miss {
			key = "-"
		}
		fmt.Fprintf(&b, "%-38s %-34s %-10s %-14s %s\n", r.input, key, r.ctx, r.mode, flag)
	}

	// Collisions: distinct inputs -> same key.
	b.WriteString("\n=== COLLISIONS (distinct inputs sharing one key) ===\n")
	collisionGroups := 0
	keys := make([]string, 0, len(keyToInputs))
	for k := range keyToInputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if ins := keyToInputs[k]; len(ins) > 1 {
			collisionGroups++
			fmt.Fprintf(&b, "  %s  <=  %s\n", k, strings.Join(ins, " , "))
		}
	}
	if collisionGroups == 0 {
		b.WriteString("  (none)\n")
	}

	fmt.Fprintf(&b, "\nSUMMARY: inputs=%d matched=%d missed=%d variant-collapse=%d collision-groups=%d\n",
		len(inputs), matched, missed, collapsed, collisionGroups)

	t.Log(b.String())
}
