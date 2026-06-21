package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/config"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

func TestBuiltinProviderNilService(t *testing.T) {
	t.Parallel()
	p := NewBuiltinProvider(slog.Default(), nil, nil, nil)
	if p.Type() != BuiltinType {
		t.Fatalf("expected type %q, got %q", BuiltinType, p.Type())
	}

	result, err := p.OnBeforeChat(context.Background(), adapters.BeforeChatRequest{
		BotID: "bot-1",
		Query: "hello",
	})
	if err != nil {
		t.Fatalf("OnBeforeChat error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for nil service, got %+v", result)
	}
}

func TestBuiltinProviderSemanticCompactCapability(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	runtime := newFileRuntime(store)
	p := NewBuiltinProvider(slog.Default(), runtime, nil, nil)

	withoutLLM := p.SemanticCompactCapability()
	if withoutLLM.Semantic {
		t.Fatal("semantic compact should be unavailable without an LLM")
	}
	if withoutLLM.Reason == "" {
		t.Fatal("expected unavailable semantic compact to explain why")
	}

	p.SetLLM(&fakeLLM{})
	withLLM := p.SemanticCompactCapability()
	if !withLLM.Semantic {
		t.Fatalf("semantic compact should be available with LLM and runtime support: %+v", withLLM)
	}
	if !withLLM.Archive {
		t.Fatalf("semantic compact should advertise source archive support: %+v", withLLM)
	}
	// The file runtime (mode "off") has no derived vector index to rebuild,
	// so RebuildIndex must be false. Only indexed runtimes (dense) set it.
	if withLLM.RebuildIndex {
		t.Fatalf("file runtime should not advertise index rebuild support: %+v", withLLM)
	}
	if withLLM.Reason != "" {
		t.Fatalf("available semantic compact should not include unavailable reason: %+v", withLLM)
	}
}

func TestBuiltinProviderOnBeforeChatEmptyQuery(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	runtime := newFileRuntime(store)
	p := NewBuiltinProvider(slog.Default(), runtime, nil, nil)

	result, err := p.OnBeforeChat(context.Background(), adapters.BeforeChatRequest{
		BotID: "bot-1",
		Query: "",
	})
	if err != nil {
		t.Fatalf("OnBeforeChat error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for empty query")
	}
}

func TestBuiltinProviderContextPackingProducesMemoryContextTags(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	runtime := newFileRuntime(store)
	p := NewBuiltinProvider(slog.Default(), runtime, nil, nil)

	_ = p.OnAfterChat(context.Background(), adapters.AfterChatRequest{
		BotID:    "bot-1",
		Messages: []adapters.Message{{Role: "user", Content: "I like green tea"}},
	})
	_ = p.OnAfterChat(context.Background(), adapters.AfterChatRequest{
		BotID:    "bot-1",
		Messages: []adapters.Message{{Role: "user", Content: "I work in Tokyo"}},
	})

	result, err := p.OnBeforeChat(context.Background(), adapters.BeforeChatRequest{
		BotID: "bot-1",
		Query: "tea",
	})
	if err != nil {
		t.Fatalf("OnBeforeChat error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
		return
	}
	if !strings.Contains(result.ContextText, "<memory-context>") {
		t.Fatalf("expected memory-context tags, got: %s", result.ContextText)
	}
	if !strings.Contains(result.ContextText, "</memory-context>") {
		t.Fatalf("expected closing memory-context tag, got: %s", result.ContextText)
	}
}

func TestBuiltinProviderApplyProviderConfig(t *testing.T) {
	t.Parallel()
	p := NewBuiltinProvider(slog.Default(), nil, nil, nil)

	p.ApplyProviderConfig(map[string]any{
		"context_target_items":    float64(10),
		"context_max_total_chars": float64(3000),
	})

	if p.packer.TargetItems != 10 {
		t.Fatalf("expected TargetItems=10, got %d", p.packer.TargetItems)
	}
	if p.packer.MaxTotalChars != 3000 {
		t.Fatalf("expected MaxTotalChars=3000, got %d", p.packer.MaxTotalChars)
	}
	if p.packer.MinItemChars != defaultPackerConfig.MinItemChars {
		t.Fatalf("expected MinItemChars to remain default, got %d", p.packer.MinItemChars)
	}
}

func TestBuiltinProviderApplyProviderConfigNil(t *testing.T) {
	t.Parallel()
	p := NewBuiltinProvider(slog.Default(), nil, nil, nil)
	p.ApplyProviderConfig(nil)
	if p.packer.TargetItems != defaultPackerConfig.TargetItems {
		t.Fatalf("expected default TargetItems, got %d", p.packer.TargetItems)
	}
}

func TestBuiltinProviderCompactUsesLLMResults(t *testing.T) {
	t.Parallel()
	store := newFakeStore(
		storefs.MemoryItem{ID: "bot-1:mem_1", Memory: "Ran likes black tea", CreatedAt: "2026-06-01T00:00:00Z", UpdatedAt: "2026-06-01T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_2", Memory: "Ran likes oolong tea", CreatedAt: "2026-06-02T00:00:00Z", UpdatedAt: "2026-06-02T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_3", Memory: "Ran works in Berlin", CreatedAt: "2026-06-03T00:00:00Z", UpdatedAt: "2026-06-03T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_4", Memory: "Ran uses Vim", CreatedAt: "2026-06-04T00:00:00Z", UpdatedAt: "2026-06-04T00:00:00Z"},
	)
	runtime := newFileRuntime(store)
	llm := &fakeLLM{
		compactFacts: []string{
			"Ran likes tea, especially black tea and oolong.",
			"Ran works in Berlin.",
		},
	}
	provider := NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	provider.SetLLM(llm)

	result, err := provider.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.5, 30)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if llm.compactCalls != 1 {
		t.Fatalf("expected LLM compact to be called once, got %d", llm.compactCalls)
	}
	if len(llm.compactReqs) != 1 {
		t.Fatalf("expected one compact request, got %d", len(llm.compactReqs))
	}
	req := llm.compactReqs[0]
	if req.TargetCount != 2 {
		t.Fatalf("expected target_count=2, got %d", req.TargetCount)
	}
	if req.DecayDays != 30 {
		t.Fatalf("expected decay_days=30, got %d", req.DecayDays)
	}
	if len(req.Memories) != 4 {
		t.Fatalf("expected 4 candidate memories, got %d", len(req.Memories))
	}
	if result.BeforeCount != 4 || result.AfterCount != 2 {
		t.Fatalf("unexpected counts: before=%d after=%d", result.BeforeCount, result.AfterCount)
	}
	got := map[string]bool{}
	for _, item := range store.items {
		got[item.Memory] = true
	}
	for _, fact := range llm.compactFacts {
		if !got[fact] {
			t.Fatalf("expected compacted fact %q in store, got %#v", fact, got)
		}
	}
	if len(store.archive) != 4 {
		t.Fatalf("expected 4 archived source memories, got %d", len(store.archive))
	}
	for _, item := range store.archive {
		if item.Metadata["compacted_at"] == nil {
			t.Fatalf("expected archived memory %s to record compacted_at metadata: %#v", item.ID, item.Metadata)
		}
		if item.Metadata["superseded_by"] == nil {
			t.Fatalf("expected archived memory %s to record superseded_by metadata: %#v", item.ID, item.Metadata)
		}
	}
}

func TestBuiltinProviderCompactRequiresSemanticCompactCapability(t *testing.T) {
	t.Parallel()
	store := newFakeStore(
		storefs.MemoryItem{ID: "bot-1:mem_1", Memory: "Ran likes black tea", CreatedAt: "2026-06-01T00:00:00Z", UpdatedAt: "2026-06-01T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_2", Memory: "Ran likes oolong tea", CreatedAt: "2026-06-02T00:00:00Z", UpdatedAt: "2026-06-02T00:00:00Z"},
	)
	runtime := newFileRuntime(store)
	provider := NewBuiltinProvider(slog.Default(), runtime, nil, nil)

	if _, err := provider.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.5, 0); err == nil {
		t.Fatal("expected compact without LLM to fail instead of truncating memories")
	}
}

func TestBuiltinProviderCompactPreservesPinnedMemories(t *testing.T) {
	t.Parallel()
	store := newFakeStore(
		storefs.MemoryItem{ID: "bot-1:mem_1", Memory: "Pinned preference", Metadata: map[string]any{"pinned": true}, CreatedAt: "2026-06-01T00:00:00Z", UpdatedAt: "2026-06-01T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_2", Memory: "Read-only profile", Metadata: map[string]any{"read_only": "true"}, CreatedAt: "2026-06-02T00:00:00Z", UpdatedAt: "2026-06-02T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_3", Memory: "Ran likes green tea", CreatedAt: "2026-06-03T00:00:00Z", UpdatedAt: "2026-06-03T00:00:00Z"},
		storefs.MemoryItem{ID: "bot-1:mem_4", Memory: "Ran likes oolong tea", CreatedAt: "2026-06-04T00:00:00Z", UpdatedAt: "2026-06-04T00:00:00Z"},
	)
	runtime := newFileRuntime(store)
	llm := &fakeLLM{compactFacts: []string{"Ran likes tea."}}
	provider := NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	provider.SetLLM(llm)

	result, err := provider.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.5, 0)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if result.BeforeCount != 4 || result.AfterCount != 3 {
		t.Fatalf("unexpected counts: before=%d after=%d", result.BeforeCount, result.AfterCount)
	}
	if len(llm.compactReqs) != 1 || len(llm.compactReqs[0].Memories) != 2 {
		t.Fatalf("expected only 2 compactable memories sent to LLM, got %#v", llm.compactReqs)
	}
	if _, ok := store.items["bot-1:mem_1"]; !ok {
		t.Fatal("expected pinned memory to remain active")
	}
	if _, ok := store.items["bot-1:mem_2"]; !ok {
		t.Fatal("expected read-only memory to remain active")
	}
	if len(store.archive) != 2 {
		t.Fatalf("expected only compactable memories archived, got %d", len(store.archive))
	}
}

func TestBuiltinProviderCompactBatchesOversizedInputs(t *testing.T) {
	t.Parallel()
	items := make([]storefs.MemoryItem, 0, 24)
	for i := 0; i < 24; i++ {
		items = append(items, storefs.MemoryItem{
			ID:        fmt.Sprintf("bot-1:mem_%02d", i),
			Memory:    fmt.Sprintf("memory %02d %s", i, strings.Repeat("x", 1600)),
			CreatedAt: "2026-06-01T00:00:00Z",
			UpdatedAt: "2026-06-01T00:00:00Z",
		})
	}
	store := newFakeStore(items...)
	runtime := newFileRuntime(store)
	llm := &fakeLLM{}
	llm.compactFunc = func(_ adapters.CompactRequest) adapters.CompactResponse {
		return adapters.CompactResponse{Facts: []string{
			fmt.Sprintf("summary call %02d a", llm.compactCalls),
			fmt.Sprintf("summary call %02d b", llm.compactCalls),
			fmt.Sprintf("summary call %02d c", llm.compactCalls),
		}}
	}
	provider := NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	provider.SetLLM(llm)

	result, err := provider.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.25, 0)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if llm.compactCalls < 2 {
		t.Fatalf("expected oversized input to be compacted in batches, got %d call(s)", llm.compactCalls)
	}
	for _, req := range llm.compactReqs {
		if chars := compactCandidateChars(req.Memories); chars > compactMaxCandidateChars && len(req.Memories) > 1 {
			t.Fatalf("compact request exceeded budget: chars=%d memories=%d", chars, len(req.Memories))
		}
	}
	if result.AfterCount > 6 {
		t.Fatalf("expected final compacted count <= 6, got %d", result.AfterCount)
	}
	if len(store.archive) != 24 {
		t.Fatalf("expected all source memories archived, got %d", len(store.archive))
	}
}

func TestIntFromConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		m        map[string]any
		key      string
		expected int
	}{
		{"float64", map[string]any{"k": float64(42)}, "k", 42},
		{"int", map[string]any{"k": 10}, "k", 10},
		{"missing", map[string]any{}, "k", 0},
		{"nil_map", nil, "k", 0},
		{"string_value", map[string]any{"k": "abc"}, "k", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := intFromConfig(tc.m, tc.key)
			if got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestBuiltinProviderCRUDErrorsWithNilService(t *testing.T) {
	t.Parallel()
	p := NewBuiltinProvider(slog.Default(), nil, nil, nil)
	if _, err := p.Add(context.Background(), adapters.AddRequest{}); err == nil {
		t.Fatal("expected Add error")
	}
	if _, err := p.GetAll(context.Background(), adapters.GetAllRequest{}); err == nil {
		t.Fatal("expected GetAll error")
	}
	if _, err := p.Update(context.Background(), adapters.UpdateRequest{}); err == nil {
		t.Fatal("expected Update error")
	}
	if _, err := p.Delete(context.Background(), "x"); err == nil {
		t.Fatal("expected Delete error")
	}
	if _, err := p.DeleteBatch(context.Background(), []string{"x"}); err == nil {
		t.Fatal("expected DeleteBatch error")
	}
	if _, err := p.DeleteAll(context.Background(), adapters.DeleteAllRequest{}); err == nil {
		t.Fatal("expected DeleteAll error")
	}
	if _, err := p.Compact(context.Background(), nil, 0.5, 0); err == nil {
		t.Fatal("expected Compact error")
	}
	if _, err := p.Usage(context.Background(), nil); err == nil {
		t.Fatal("expected Usage error")
	}
	if _, err := p.Status(context.Background(), "b"); err == nil {
		t.Fatal("expected Status error")
	}
	if _, err := p.Rebuild(context.Background(), "b"); err == nil {
		t.Fatal("expected Rebuild error")
	}
}

func TestNewBuiltinRuntimeFromConfig_DefaultIsGraphRequiringWikiStore(t *testing.T) {
	t.Parallel()
	// Default mode is now graph, so without a wiki store it must error.
	if _, err := NewBuiltinRuntimeFromConfig(nil, nil, nil, nil, defaultTestConfig(), nil); err == nil {
		t.Fatal("expected error for default (graph) mode without wiki store")
	}
}

func TestNewBuiltinRuntimeFromConfig_DenseErrorPropagates(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{"memory_mode": "dense"}
	_, err := NewBuiltinRuntimeFromConfig(nil, cfg, nil, nil, defaultTestConfig(), nil)
	if err == nil {
		t.Fatal("expected error for dense mode without embedding_model_id")
	}
}

func TestNewBuiltinRuntimeFromConfig_GraphRequiresWikiStore(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{"memory_mode": "graph"}
	// No wiki store -> error.
	if _, err := NewBuiltinRuntimeFromConfig(nil, cfg, nil, nil, defaultTestConfig(), nil); err == nil {
		t.Fatal("expected error for graph mode without wiki store")
	}
}

func defaultTestConfig() config.Config {
	return config.Config{}
}
