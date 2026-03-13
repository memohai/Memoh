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
			Metadata:  map[string]any{"topic": "Notes"},
		},
		{
			ID:        "mem_1",
			Memory:    "first record",
			Hash:      "h1",
			CreatedAt: "2026-03-01T09:40:00Z",
			Metadata:  map[string]any{"topic": "Decision"},
		},
	}

	md := formatMemoryDayMD("2026-03-01", items)
	if !strings.Contains(md, "# Memory 2026-03-01") {
		t.Fatalf("expected header in markdown: %s", md)
	}
	if !strings.Contains(md, "## Entry mem_1") {
		t.Fatalf("expected entry heading in markdown: %s", md)
	}
	if !strings.Contains(md, "```yaml") {
		t.Fatalf("expected yaml block in markdown: %s", md)
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
	if got := parsed[0].Metadata["topic"]; got != "Decision" {
		t.Fatalf("expected metadata preserved, got %#v", parsed[0].Metadata)
	}
}

func TestParseJSONMemoryItems(t *testing.T) {
	raw := `[
  {
    "id": "mem_json",
    "topic": "Decision",
    "memory": "Choose provider architecture."
  }
]`

	items, err := parseJSONMemoryItems(raw)
	if err != nil {
		t.Fatalf("parseJSONMemoryItems error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "mem_json" {
		t.Fatalf("unexpected id: %#v", items[0])
	}
	if got := items[0].Metadata["topic"]; got != "Decision" {
		t.Fatalf("expected topic metadata, got %#v", items[0].Metadata)
	}
	if items[0].Memory != "Choose provider architecture." {
		t.Fatalf("unexpected memory body: %#v", items[0])
	}
}

func TestParseJSONMemoryItemsCanBeFormattedToCanonicalMarkdown(t *testing.T) {
	raw := `[
  {
    "id": "mem_json",
    "topic": "Decision",
    "memory": "Choose provider architecture."
  }
]`

	items, err := parseJSONMemoryItems(raw)
	if err != nil {
		t.Fatalf("parseJSONMemoryItems error: %v", err)
	}
	md := formatMemoryDayMD("2026-03-01", items)
	if !strings.Contains(md, "## Entry mem_json") {
		t.Fatalf("expected canonical heading, got: %s", md)
	}
	if !strings.Contains(md, "topic: Decision") {
		t.Fatalf("expected yaml metadata topic, got: %s", md)
	}
	parsed, err := parseMemoryDayMD(md)
	if err != nil {
		t.Fatalf("parseMemoryDayMD error: %v", err)
	}
	if len(parsed) != 1 || parsed[0].ID != "mem_json" {
		t.Fatalf("unexpected parsed canonical items: %#v", parsed)
	}
}
