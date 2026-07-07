package compaction

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestBuildEntriesAndIDsMarksOnlyRenderedItemsInMixedWindow(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"first question"`, 0),
		// reasoning-only: renders to empty and must NOT add a bare
		// "assistant:" line or be marked compacted in a mixed window.
		mkRow(t, "assistant", `[{"type":"reasoning","text":"thinking"}]`, 0),
		mkRow(t, "assistant", `"the answer"`, 0),
	}
	items, _ := itemsFromRows(rows)

	entries, ids := buildEntriesAndIDs(items)

	if len(ids) != 2 {
		t.Fatalf("ids = %d, want 2 (only rendered rows marked)", len(ids))
	}
	wantIDs := []int{0, 2}
	for i, rowIdx := range wantIDs {
		if ids[i] != rows[rowIdx].ID {
			t.Fatalf("id[%d] misaligned with rendered row %d", i, rowIdx)
		}
	}

	// The reasoning-only message contributes no summarizer entry.
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (empty entry skipped)", len(entries))
	}
	if entries[0].Content != "first question" || entries[1].Content != "the answer" {
		t.Fatalf("entries lost order/content: %+v", entries)
	}
	for _, e := range entries {
		if strings.TrimSpace(e.Content) == "" {
			t.Fatalf("emitted an empty entry")
		}
	}
}

func TestBuildEntriesAndIDsWithholdsWholeExchangeWhenResultRendersEmpty(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"question"`, 0),
		toolCallRow(t, 0),
		// degenerate content: still a "tool" role closure row, but has no
		// recognizable parts, so it renders empty.
		mkRow(t, "tool", `[]`, 0),
	}
	items, _ := itemsFromRows(rows)

	entries, ids := buildEntriesAndIDs(items)

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (question + rendered call marker)", len(entries))
	}
	if len(ids) != 1 || ids[0] != rows[0].ID {
		t.Fatalf("ids = %#v, want only the unrelated question row marked", ids)
	}
}

func TestBuildUserPromptPriorContextBranch(t *testing.T) {
	t.Parallel()

	entries := []messageEntry{{Role: "user", Content: "hi"}}

	withPrior := buildUserPrompt([]string{"earlier summary"}, entries)
	if !strings.Contains(withPrior, "<prior_context>") || !strings.Contains(withPrior, "earlier summary") {
		t.Fatalf("prior-summary branch not rendered: %q", withPrior)
	}
	if !strings.Contains(withPrior, "user: hi") {
		t.Fatalf("entries not included with prior context: %q", withPrior)
	}

	without := buildUserPrompt(nil, entries)
	if strings.Contains(without, "<prior_context>") {
		t.Fatalf("no-prior branch should omit prior context: %q", without)
	}
	if !strings.Contains(without, "Summarize the following conversation:") {
		t.Fatalf("no-prior branch missing lead-in: %q", without)
	}
}
