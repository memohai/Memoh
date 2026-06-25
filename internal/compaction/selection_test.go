package compaction

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func testUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func mkRow(t *testing.T, role, content string, outputTokens int) sqlc.ListUncompactedMessagesBySessionRow {
	t.Helper()
	row := sqlc.ListUncompactedMessagesBySessionRow{
		ID:      testUUID(t),
		Role:    role,
		Content: json.RawMessage(content),
	}
	if outputTokens > 0 {
		row.Usage = json.RawMessage(`{"output_tokens":` + strconv.Itoa(outputTokens) + `}`)
	}
	return row
}

func itemTokens(items []compactionItem) []int {
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = estimateItemTokens(it)
	}
	return out
}

func TestEstimateItemTokensPrefersOutputTokensThenContentFallback(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"hello"`, 50),
		mkRow(t, "assistant", `"`+repeat("a", 400)+`"`, 0),
	}
	items, _ := itemsFromRows(rows)
	got := itemTokens(items)
	// First: usage output_tokens. Second: len(content)/4 = 402/4 = 100.
	if got[0] != 50 {
		t.Fatalf("usage token estimate = %d, want 50", got[0])
	}
	if got[1] != len(items[1].content)/4 {
		t.Fatalf("content token estimate = %d, want %d", got[1], len(items[1].content)/4)
	}
}

func TestItemsFromRowsPreservesIDContentAndClassifiesRecord(t *testing.T) {
	t.Parallel()

	row := mkRow(t, "user", `"hi there"`, 0)
	items, _ := itemsFromRows([]sqlc.ListUncompactedMessagesBySessionRow{row})
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	it := items[0]
	if it.id != row.ID {
		t.Fatalf("item id mismatch")
	}
	if string(it.content) != string(row.Content) {
		t.Fatalf("item content = %q, want %q", it.content, row.Content)
	}
	if it.record.ModelMessage.Role != "user" {
		t.Fatalf("record role = %q, want user", it.record.ModelMessage.Role)
	}
	if it.record.ModelMessage.TextContent() != "hi there" {
		t.Fatalf("record text = %q, want 'hi there'", it.record.ModelMessage.TextContent())
	}
}

func TestItemsFromRowsSkipsUnparseableRowsWithoutAborting(t *testing.T) {
	t.Parallel()

	good := mkRow(t, "user", `"ok"`, 0)
	bad := sqlc.ListUncompactedMessagesBySessionRow{
		ID:      pgtype.UUID{Valid: false}, // empty id -> FromDBMessage rejects it
		Role:    "user",
		Content: json.RawMessage(`"x"`),
	}
	items, skipped := itemsFromRows([]sqlc.ListUncompactedMessagesBySessionRow{good, bad})
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
	if len(items) != 1 || items[0].id != good.ID {
		t.Fatalf("a bad row must be skipped, not abort the batch: got %d items", len(items))
	}
}

func TestSplitByTargetCompactsOldestBeyondTarget(t *testing.T) {
	t.Parallel()

	// Tokens (oldest -> newest): 100, 100, 100. Target 150: newest fits (100),
	// adding the next exceeds 150, so keep the newest one and compact the oldest two.
	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"a"`, 100),
		mkRow(t, "assistant", `"b"`, 100),
		mkRow(t, "user", `"c"`, 100),
	}
	items, _ := itemsFromRows(rows)
	toCompact := splitByTarget(items, 150)
	if len(toCompact) != 2 {
		t.Fatalf("compact count = %d, want 2", len(toCompact))
	}
	if toCompact[0].id != rows[0].ID || toCompact[1].id != rows[1].ID {
		t.Fatalf("compacted the wrong (non-oldest) messages")
	}
}

func TestSplitByRatioKeepsNewestByRatio(t *testing.T) {
	t.Parallel()

	// total=300, ratio=50 -> keepTokens=150 -> keep newest one, compact oldest two.
	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"a"`, 100),
		mkRow(t, "assistant", `"b"`, 100),
		mkRow(t, "user", `"c"`, 100),
	}
	items, _ := itemsFromRows(rows)
	toCompact := splitByRatio(items, 300, 50)
	if len(toCompact) != 2 {
		t.Fatalf("compact count = %d, want 2", len(toCompact))
	}
	if toCompact[0].id != rows[0].ID || toCompact[1].id != rows[1].ID {
		t.Fatalf("compacted the wrong messages")
	}
}

func TestSplitByRatioBoundaryConditions(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{mkRow(t, "user", `"a"`, 100)}
	items, _ := itemsFromRows(rows)

	if got := splitByRatio(items, 0, 50); got != nil {
		t.Fatalf("zero total tokens should yield nil, got %d", len(got))
	}
	if got := splitByRatio(items, 100, 0); got != nil {
		t.Fatalf("zero ratio should yield nil, got %d", len(got))
	}
	if got := splitByRatio(items, 100, 100); len(got) != 1 {
		t.Fatalf("ratio 100 should compact all, got %d", len(got))
	}
}

func TestTrimCompactMessagesDropsOldestBeyondBudget(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"a"`, 100),
		mkRow(t, "assistant", `"b"`, 100),
		mkRow(t, "user", `"c"`, 100),
	}
	items, _ := itemsFromRows(rows)
	trimmed := trimCompactMessages(items, 150)
	// Budget 150, accumulate from newest: 100 (c) fits, +100 (b) = 200 > 150 -> drop
	// the oldest two (a, b) from the tail, keep only the newest (c).
	if len(trimmed) != 1 {
		t.Fatalf("trimmed count = %d, want 1", len(trimmed))
	}
	if trimmed[0].id != rows[2].ID {
		t.Fatalf("trim should keep the newest message and drop older ones")
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
