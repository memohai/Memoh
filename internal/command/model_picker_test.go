package command

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/i18n"
)

// TestFormatProvidersSummary pins the text body that no-button channels see
// when /model is invoked without a provider filter. It mirrors Telegram's
// LevelProviders picker: title with total count, optional current-model line,
// optional reasoning line, then one bullet per provider with count and a ●
// mark on the provider that owns the active model.
func TestFormatProvidersSummary(t *testing.T) {
	cc := CommandContext{L: i18n.New("en")}
	cands := []modelCandidate{
		{dbID: "d1", name: "DS-V4", provider: "DeepSeek"},
		{dbID: "d2", name: "DS-V3", provider: "DeepSeek"},
		{dbID: "o1", name: "gpt-4o", provider: "OpenAI"},
	}
	groups := []providerGroup{
		{name: "DeepSeek", modelIdx: []int{0, 1}},
		{name: "OpenAI", modelIdx: []int{2}},
	}

	t.Run("happy path", func(t *testing.T) {
		got := formatProvidersSummary(cc, groups, cands, "d1", "DS-V4 (DeepSeek)", "medium")
		// Title with total count.
		if !strings.Contains(got, "Chat Models") {
			t.Errorf("missing title: %s", got)
		}
		if !strings.Contains(got, "(3)") {
			t.Errorf("missing total count (3): %s", got)
		}
		// Current model header.
		if !strings.Contains(got, "DS-V4 (DeepSeek)") {
			t.Errorf("missing current model: %s", got)
		}
		// Reasoning line.
		if !strings.Contains(got, "medium") {
			t.Errorf("missing reasoning: %s", got)
		}
		// Provider bullets with counts.
		if !strings.Contains(got, "- DeepSeek (2)") {
			t.Errorf("missing DeepSeek bullet with count: %s", got)
		}
		if !strings.Contains(got, "- OpenAI (1)") {
			t.Errorf("missing OpenAI bullet with count: %s", got)
		}
		// Current-provider marker on DeepSeek (holds d1).
		if !strings.Contains(got, "DeepSeek (2) ●") {
			t.Errorf("DeepSeek should carry the ● marker (holds current model d1): %s", got)
		}
		// OpenAI must NOT carry the marker.
		if strings.Contains(got, "OpenAI (1) ●") {
			t.Errorf("OpenAI should not carry the ● marker: %s", got)
		}
	})

	t.Run("no current model", func(t *testing.T) {
		got := formatProvidersSummary(cc, groups, cands, "", "", "")
		if strings.Contains(got, "Current model:") {
			t.Errorf("current-model line should be absent when display is empty: %s", got)
		}
		if strings.Contains(got, "Reasoning:") {
			t.Errorf("reasoning line should be absent when reasoning is empty: %s", got)
		}
		// No provider should carry the ● marker without a current model.
		if strings.Contains(got, "●") {
			t.Errorf("no provider should be marked active when no current model: %s", got)
		}
	})

	t.Run("no reasoning", func(t *testing.T) {
		got := formatProvidersSummary(cc, groups, cands, "d1", "DS-V4", "")
		if strings.Contains(got, "Reasoning:") {
			t.Errorf("reasoning line should be absent: %s", got)
		}
		if !strings.Contains(got, "DS-V4") {
			t.Errorf("current model line should still be present: %s", got)
		}
	})

	t.Run("total counts sum of group sizes", func(t *testing.T) {
		// Single group with 5 models — total should be 5.
		soloGroups := []providerGroup{{name: "Solo", modelIdx: []int{0, 1, 2, 3, 4}}}
		soloCands := make([]modelCandidate, 5)
		got := formatProvidersSummary(cc, soloGroups, soloCands, "", "", "")
		if !strings.Contains(got, "(5)") {
			t.Errorf("title should reflect total=5: %s", got)
		}
	})

	t.Run("zh locale", func(t *testing.T) {
		zh := CommandContext{L: i18n.New("zh")}
		got := formatProvidersSummary(zh, groups, cands, "d1", "DS-V4", "medium")
		// 对话模型 + 按服务商 + 当前模型 + 推理 should all be in zh.
		for _, want := range []string{"对话模型", "按服务商", "当前模型", "推理"} {
			if !strings.Contains(got, want) {
				t.Errorf("zh trailer missing %q: %s", want, got)
			}
		}
		// Provider names and counts stay verbatim.
		if !strings.Contains(got, "DeepSeek (2) ●") {
			t.Errorf("provider line should survive zh locale: %s", got)
		}
	})

	t.Run("trailing newline trimmed", func(t *testing.T) {
		got := formatProvidersSummary(cc, groups, cands, "", "", "")
		if strings.HasSuffix(got, "\n") {
			t.Errorf("output should not end with newline: %q", got)
		}
	})
}
