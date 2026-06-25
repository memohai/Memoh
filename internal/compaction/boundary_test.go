package compaction

import (
	"testing"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func toolCallRow(t *testing.T, tokens int) sqlc.ListUncompactedMessagesBySessionRow {
	t.Helper()
	return mkRow(t, "assistant", `[{"type":"tool-call","toolName":"calc","toolCallId":"c1","input":{}}]`, tokens)
}

func toolResultRow(t *testing.T, tokens int) sqlc.ListUncompactedMessagesBySessionRow {
	t.Helper()
	return mkRow(t, "tool", `[{"type":"tool-result","toolName":"calc","toolCallId":"c1","output":{"type":"text","value":"42"}}]`, tokens)
}

func firstKeptIsNotOrphanTool(t *testing.T, items, toCompact []compactionItem) {
	t.Helper()
	keepStart := len(toCompact)
	if keepStart < len(items) && isToolResultItem(items[keepStart]) {
		t.Fatalf("keep side starts with an orphan tool result at index %d", keepStart)
	}
}

func TestSplitByTargetDoesNotOrphanToolResult(t *testing.T) {
	t.Parallel()

	// oldest -> newest: user, assistant(tool-call), tool(result), assistant.
	// target 250 would otherwise cut between the tool call and its result,
	// leaving the kept side starting with an orphan tool result.
	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"q"`, 100),
		toolCallRow(t, 100),
		toolResultRow(t, 100),
		mkRow(t, "assistant", `"done"`, 100),
	}
	items, err := itemsFromRows(rows)
	if err != nil {
		t.Fatalf("itemsFromRows: %v", err)
	}
	toCompact := splitByTarget(items, 250)
	firstKeptIsNotOrphanTool(t, items, toCompact)
	if len(toCompact) != 3 {
		t.Fatalf("compact count = %d, want 3 (tool result pulled into compact set)", len(toCompact))
	}
	if !isToolResultItem(toCompact[2]) {
		t.Fatalf("last compacted message should be the tool result")
	}
}

func TestSplitByRatioDoesNotOrphanToolResult(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"q"`, 100),
		toolCallRow(t, 100),
		toolResultRow(t, 100),
		mkRow(t, "assistant", `"done"`, 100),
	}
	items, err := itemsFromRows(rows)
	if err != nil {
		t.Fatalf("itemsFromRows: %v", err)
	}
	// total=500, ratio=50 -> keepTokens=250 -> unadjusted cutoff would keep the
	// tool result at the head of the keep side.
	toCompact := splitByRatio(items, 500, 50)
	firstKeptIsNotOrphanTool(t, items, toCompact)
	if len(toCompact) != 3 {
		t.Fatalf("compact count = %d, want 3", len(toCompact))
	}
}

func TestAdjustForToolBoundaryNoOpWhenKeepStartsClean(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"a"`, 100),
		mkRow(t, "assistant", `"b"`, 100),
		mkRow(t, "user", `"c"`, 100),
	}
	items, _ := itemsFromRows(rows)
	if got := adjustForToolBoundary(items, 2); got != 2 {
		t.Fatalf("clean boundary should be unchanged, got %d", got)
	}
}

func TestAdjustForToolBoundaryAdvancesPastConsecutiveResults(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		toolCallRow(t, 100),
		toolResultRow(t, 100),
		toolResultRow(t, 100),
		mkRow(t, "assistant", `"done"`, 100),
	}
	items, _ := itemsFromRows(rows)
	// cutoff=1 would keep two consecutive tool results; advance past both.
	if got := adjustForToolBoundary(items, 1); got != 3 {
		t.Fatalf("should advance past consecutive tool results, got %d", got)
	}
}

func TestAdjustForToolBoundaryAllToolsCompactsAll(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		toolResultRow(t, 100),
		toolResultRow(t, 100),
	}
	items, _ := itemsFromRows(rows)
	if got := adjustForToolBoundary(items, 1); got != 2 {
		t.Fatalf("all-tool tail should compact everything, got %d", got)
	}
}

func TestAdjustForToolBoundaryIgnoresZeroCutoff(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		toolResultRow(t, 100),
		mkRow(t, "assistant", `"x"`, 100),
	}
	items, _ := itemsFromRows(rows)
	// cutoff 0 means "compact nothing"; never start compacting from the front.
	if got := adjustForToolBoundary(items, 0); got != 0 {
		t.Fatalf("zero cutoff must stay zero, got %d", got)
	}
}
