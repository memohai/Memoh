package compaction

import (
	"strings"
	"testing"
)

func TestCapPriorSummariesKeepsNewestWithinBudget(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("x", 400) // ~100 tokens each
	summaries := []string{"oldest " + big, "middle " + big, "newest " + big}

	capped := capPriorSummaries(summaries, 220)
	if len(capped) != 2 {
		t.Fatalf("capped = %d summaries, want 2", len(capped))
	}
	if !strings.HasPrefix(capped[0], "middle") || !strings.HasPrefix(capped[1], "newest") {
		t.Fatalf("cap must keep the newest summaries in chronological order, got %q", capped)
	}
}

func TestCapPriorSummariesAlwaysKeepsTheNewestTruncatedToBudget(t *testing.T) {
	t.Parallel()

	summaries := []string{"old", strings.Repeat("y", 4000)}
	capped := capPriorSummaries(summaries, 10)
	if len(capped) != 1 || !strings.HasPrefix(capped[0], "y") {
		t.Fatalf("cap must keep at least the newest summary, got %q", capped)
	}
	if got := estimateBytesAsTokens(capped[0]) + priorSeparatorTokens; got > 10 {
		t.Fatalf("the cap is a hard bound, marker included: got ~%d tokens for cap 10", got)
	}
}

func TestCapPriorSummariesDropsAllOnNonPositiveBudget(t *testing.T) {
	t.Parallel()

	if got := capPriorSummaries([]string{"a", "b"}, 0); got != nil {
		t.Fatalf("non-positive budget must drop all prior context, got %q", got)
	}
	if got := capPriorSummaries([]string{"a", "b"}, -5); got != nil {
		t.Fatalf("negative budget must drop all prior context, got %q", got)
	}
}

func TestBuildUserPromptOrdersPriorContextChronologically(t *testing.T) {
	t.Parallel()

	prompt := buildUserPrompt([]string{"first segment", "second segment"}, []messageEntry{{Role: "user", Content: "now"}})
	if !strings.Contains(prompt, "first segment\n---\nsecond segment") {
		t.Fatalf("prior context must stay in chronological order:\n%s", prompt)
	}
}

func TestCapEntriesToBudgetHoldsTheTotalAcrossManyEntries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		entries []messageEntry
		max     int
	}{
		{"two oversized", []messageEntry{
			{Role: "user", Content: strings.Repeat("a", 2400)},
			{Role: "assistant", Content: strings.Repeat("b", 2400)},
		}, 1000},
		{"one giant many small", []messageEntry{
			{Role: "user", Content: strings.Repeat("a", 4800)},
			{Role: "assistant", Content: strings.Repeat("b", 40)},
			{Role: "user", Content: strings.Repeat("c", 40)},
			{Role: "assistant", Content: strings.Repeat("d", 40)},
		}, 1000},
		{"tight budget", []messageEntry{
			{Role: "user", Content: strings.Repeat("a", 400)},
			{Role: "tool", Content: strings.Repeat("b", 400)},
			{Role: "assistant", Content: strings.Repeat("c", 400)},
		}, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			capped := capEntriesToBudget(tc.entries, tc.max)
			if len(capped) != len(tc.entries) {
				t.Fatalf("entries dropped: got %d, want %d (ids must stay aligned)", len(capped), len(tc.entries))
			}
			total := 0
			for _, e := range capped {
				total += estimateBytesAsTokens(e.Content) + estimateBytesAsTokens(e.Role) + 1
			}
			if total > tc.max {
				t.Fatalf("capped total = %d tokens, exceeds max = %d", total, tc.max)
			}
		})
	}
}
