package builtin

import (
	"testing"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	qdrantclient "github.com/memohai/memoh/internal/memory/qdrant"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

func TestRuntimeHash(t *testing.T) {
	t.Parallel()
	h1 := runtimeHash("hello")
	h2 := runtimeHash("  hello  ")
	if h1 != h2 {
		t.Fatalf("expected trimmed strings to produce same hash, got %q vs %q", h1, h2)
	}
	h3 := runtimeHash("world")
	if h1 == h3 {
		t.Fatal("expected different hashes for different inputs")
	}
}

func TestRuntimeBotID(t *testing.T) {
	t.Parallel()
	id, err := runtimeBotID("bot-1", nil)
	if err != nil || id != "bot-1" {
		t.Fatalf("expected bot-1, got %q, err=%v", id, err)
	}

	id, err = runtimeBotID("", map[string]any{"bot_id": "bot-2"})
	if err != nil || id != "bot-2" {
		t.Fatalf("expected bot-2 from filter, got %q, err=%v", id, err)
	}

	_, err = runtimeBotID("", nil)
	if err == nil {
		t.Fatal("expected error for empty bot_id")
	}
}

func TestRuntimeBotIDFromMemoryID(t *testing.T) {
	t.Parallel()
	if got := runtimeBotIDFromMemoryID("bot-1:mem_123"); got != "bot-1" {
		t.Fatalf("expected bot-1, got %q", got)
	}
	if got := runtimeBotIDFromMemoryID("invalid"); got != "" {
		t.Fatalf("expected empty for invalid format, got %q", got)
	}
}

func TestRuntimePointID_Deterministic(t *testing.T) {
	t.Parallel()
	p1 := runtimePointID("bot-1", "mem-1")
	p2 := runtimePointID("bot-1", "mem-1")
	if p1 != p2 {
		t.Fatalf("expected deterministic point ID, got %q vs %q", p1, p2)
	}
	p3 := runtimePointID("bot-1", "mem-2")
	if p1 == p3 {
		t.Fatal("expected different point IDs for different sources")
	}
}

func TestCanonicalStoreItem(t *testing.T) {
	t.Parallel()
	item := storefs.MemoryItem{
		ID:     "  id-1  ",
		Memory: "  hello world  ",
	}
	c := canonicalStoreItem(item)
	if c.ID != "id-1" {
		t.Fatalf("expected trimmed ID, got %q", c.ID)
	}
	if c.Memory != "hello world" {
		t.Fatalf("expected trimmed memory, got %q", c.Memory)
	}
	if c.Hash == "" {
		t.Fatal("expected hash to be populated")
	}
}

func TestPayloadMatches(t *testing.T) {
	t.Parallel()
	existing := map[string]string{"memory": "hello", "bot_id": "b1"}
	expected := map[string]string{"memory": "hello", "bot_id": "b1"}
	if !payloadMatches(existing, expected) {
		t.Fatal("expected matching payloads")
	}
	expected["memory"] = "world"
	if payloadMatches(existing, expected) {
		t.Fatal("expected non-matching payloads")
	}
}

func TestResultToItem(t *testing.T) {
	t.Parallel()
	r := qdrantclient.SearchResult{
		ID:    "point-1",
		Score: 0.95,
		Payload: map[string]string{
			"source_entry_id": "mem-1",
			"memory":          "test memory",
			"hash":            "abc",
			"bot_id":          "bot-1",
			"created_at":      "2026-01-01T00:00:00Z",
			"updated_at":      "2026-01-01T00:00:00Z",
		},
	}
	item := resultToItem(r)
	if item.ID != "mem-1" {
		t.Fatalf("expected source_entry_id as ID, got %q", item.ID)
	}
	if item.Score != 0.95 {
		t.Fatalf("expected score 0.95, got %f", item.Score)
	}
	if item.Memory != "test memory" {
		t.Fatalf("expected memory, got %q", item.Memory)
	}
}

func TestStoreItemRoundTrip(t *testing.T) {
	t.Parallel()
	original := adapters.MemoryItem{
		ID:     "id-1",
		Memory: "hello world",
		Hash:   "abc",
		BotID:  "bot-1",
	}
	store := storeItemFromMemoryItem(original)
	back := memoryItemFromStore(store)
	if back.ID != original.ID || back.Memory != original.Memory || back.BotID != original.BotID {
		t.Fatalf("round-trip failed: got %+v", back)
	}
}

func TestRuntimeText_SingleMessage(t *testing.T) {
	t.Parallel()
	text := runtimeText("hello", nil)
	if text != "hello" {
		t.Fatalf("expected 'hello', got %q", text)
	}
}

func TestRuntimeText_MultipleMessages(t *testing.T) {
	t.Parallel()
	msgs := []adapters.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	text := runtimeText("", msgs)
	if text == "" {
		t.Fatal("expected non-empty text from messages")
	}
	if !contains(text, "[USER] hi") || !contains(text, "[ASSISTANT] hello") {
		t.Fatalf("unexpected text format: %q", text)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
