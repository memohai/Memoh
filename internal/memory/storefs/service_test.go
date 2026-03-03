package storefs

import (
	"strings"
	"testing"
)

func TestFormatAndParseMemoryDayMD_Roundtrip(t *testing.T) {
	items := []MemoryItem{
		{
			ID:        "mem_2",
			Memory:    "second record",
			Hash:      "h2",
			CreatedAt: "2026-03-01T11:15:00Z",
		},
		{
			ID:        "mem_1",
			Memory:    "first record",
			Hash:      "h1",
			CreatedAt: "2026-03-01T09:40:00Z",
		},
	}

	md := formatMemoryDayMD("2026-03-01", items)
	if !strings.Contains(md, "# Memory 2026-03-01") {
		t.Fatalf("expected header in markdown: %s", md)
	}

	parsed, err := parseMemoryDayMD(md)
	if err != nil {
		t.Fatalf("parseMemoryDayMD error: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed items, got %d", len(parsed))
	}
	// formatMemoryDayMD sorts by created_at ascending.
	if parsed[0].ID != "mem_1" || parsed[1].ID != "mem_2" {
		t.Fatalf("unexpected order after roundtrip: %#v", parsed)
	}
}

func TestParseLegacyMemoryMD(t *testing.T) {
	legacy := `---
id: mem_legacy
hash: legacyhash
created_at: 2026-03-01T09:00:00Z
updated_at: 2026-03-01T10:00:00Z
---
legacy content`

	item, err := parseLegacyMemoryMD(legacy)
	if err != nil {
		t.Fatalf("parseLegacyMemoryMD error: %v", err)
	}
	if item.ID != "mem_legacy" {
		t.Fatalf("unexpected id: %#v", item)
	}
	if item.Hash != "legacyhash" {
		t.Fatalf("unexpected hash: %#v", item)
	}
	if item.Memory != "legacy content" {
		t.Fatalf("unexpected memory body: %#v", item)
	}
}

