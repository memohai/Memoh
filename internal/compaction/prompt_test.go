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
	if got := estimateBytesAsTokens(capped[0]); got > 10+4 {
		t.Fatalf("forced newest summary must be truncated to the cap, got ~%d tokens", got)
	}
}

func TestBuildUserPromptOrdersPriorContextChronologically(t *testing.T) {
	t.Parallel()

	prompt := buildUserPrompt([]string{"first segment", "second segment"}, []messageEntry{{Role: "user", Content: "now"}})
	if !strings.Contains(prompt, "first segment\n---\nsecond segment") {
		t.Fatalf("prior context must stay in chronological order:\n%s", prompt)
	}
}
