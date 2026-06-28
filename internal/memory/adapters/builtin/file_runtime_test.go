package builtin

import (
	"context"
	"fmt"
	"strings"
	"testing"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

func TestFileRuntimeRejectsEmptyMemoryWithoutHTTPError(t *testing.T) {
	t.Parallel()
	runtime := newFileRuntime(newFakeStore())

	_, err := runtime.Add(context.Background(), adapters.AddRequest{BotID: "bot-1"})
	if err == nil {
		t.Fatal("Add() error = nil, want message validation error")
	}
	if strings.Contains(fmt.Sprintf("%T", err), "echo") {
		t.Fatalf("Add() returned HTTP-layer error type %T", err)
	}
	if !strings.Contains(err.Error(), "message is required") {
		t.Fatalf("Add() error = %q, want message validation", err.Error())
	}
}

func TestFileRuntimeCompactWithLLMArchivesSourceMemories(t *testing.T) {
	t.Parallel()
	store := newFakeStore(
		storefs.MemoryItem{ID: "bot-1:mem_1", Memory: "Ran likes green tea", CreatedAt: "2026-06-01T00:00:00Z", UpdatedAt: "2026-06-01T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_2", Memory: "Ran likes oolong tea", CreatedAt: "2026-06-02T00:00:00Z", UpdatedAt: "2026-06-02T00:00:00Z"},
	)
	runtime := newFileRuntime(store)
	llm := &fakeLLM{compactFacts: []string{"Ran likes tea."}}

	result, err := runtime.CompactWithLLM(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.5, 0, llm)
	if err != nil {
		t.Fatalf("CompactWithLLM() error = %v", err)
	}
	if result.BeforeCount != 2 || result.AfterCount != 1 {
		t.Fatalf("unexpected compact counts: %+v", result)
	}
	if len(store.archive) != 2 {
		t.Fatalf("archived source memories = %d, want 2", len(store.archive))
	}
}
