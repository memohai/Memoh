package builtin

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/config"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/memory/sparse"
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

func TestBuiltinProviderOnBeforeChatEmptyQuery(t *testing.T) {
	t.Parallel()
	encoder := &fakeSparseEncoder{}
	index := newFakeSparseIndex(encoder)
	store := newFakeSparseStore()
	runtime := &sparseRuntime{qdrant: index, encoder: encoder, store: store}
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
	encoder := &fakeSparseEncoder{}
	index := newFakeSparseIndex(encoder)
	store := newFakeSparseStore()
	runtime := &sparseRuntime{qdrant: index, encoder: encoder, store: store}
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

func TestBuiltinProviderBadServiceTypeDoesNotPanic(t *testing.T) {
	t.Parallel()
	p := NewBuiltinProvider(slog.Default(), "not a runtime", nil, nil)
	if p.service != nil {
		t.Fatal("expected nil service for non-memoryRuntime value")
	}
	_, err := p.Search(context.Background(), adapters.SearchRequest{BotID: "b", Query: "q"})
	if err == nil {
		t.Fatal("expected error from nil service")
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

func TestBuiltinProviderCompactUsesLLMAndReplacesWithFacts(t *testing.T) {
	t.Parallel()
	runtime := newFakeCompactRuntime([]adapters.MemoryItem{
		{ID: "bot-1:mem_old", Memory: "old unchanged memory", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "bot-1:mem_new", Memory: "new unchanged memory", CreatedAt: "2026-01-02T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z"},
		{ID: "bot-1:mem_mid", Memory: "middle unchanged memory", CreatedAt: "2026-01-03T00:00:00Z", UpdatedAt: "2026-01-03T00:00:00Z"},
	})
	llm := &fakeLLM{compactFacts: []string{"User prefers tea", "User lives in Berlin"}}
	p := NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	p.SetLLM(llm)

	result, err := p.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.5, 30)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if llm.compactCalls != 1 {
		t.Fatalf("expected one LLM compact call, got %d", llm.compactCalls)
	}
	if llm.compactReq.BotID != "bot-1" {
		t.Fatalf("expected compact BotID bot-1, got %q", llm.compactReq.BotID)
	}
	if llm.compactReq.TargetCount != 2 {
		t.Fatalf("expected target count 2, got %d", llm.compactReq.TargetCount)
	}
	if llm.compactReq.DecayDays != 30 {
		t.Fatalf("expected decay days 30, got %d", llm.compactReq.DecayDays)
	}
	if len(llm.compactReq.Memories) != 3 {
		t.Fatalf("expected 3 candidate memories, got %d", len(llm.compactReq.Memories))
	}
	if runtime.replaceCalls != 1 {
		t.Fatalf("expected one replace call, got %d", runtime.replaceCalls)
	}
	if len(runtime.replaced) != 2 {
		t.Fatalf("expected 2 replaced memories, got %d", len(runtime.replaced))
	}
	if runtime.replaced[0].Memory != "User prefers tea" || runtime.replaced[1].Memory != "User lives in Berlin" {
		t.Fatalf("expected LLM facts to replace source, got %#v", runtime.replaced)
	}
	for _, item := range runtime.replaced {
		if strings.Contains(item.Memory, "unchanged memory") {
			t.Fatalf("provider compact kept original memory instead of LLM fact: %#v", runtime.replaced)
		}
	}
	if result.BeforeCount != 3 || result.AfterCount != 2 {
		t.Fatalf("unexpected compact counts: %#v", result)
	}
}

func TestBuiltinProviderCompactTargetCountUsesNonEmptyCandidates(t *testing.T) {
	t.Parallel()
	runtime := newFakeCompactRuntime([]adapters.MemoryItem{
		{ID: "bot-1:mem_1", Memory: "one"},
		{ID: "bot-1:mem_blank", Memory: "   "},
		{ID: "bot-1:mem_2", Memory: "two"},
	})
	llm := &fakeLLM{compactFacts: []string{"one and two"}}
	p := NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	p.SetLLM(llm)

	if _, err := p.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 1, 0); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if len(llm.compactReq.Memories) != 2 {
		t.Fatalf("expected 2 candidate memories, got %d", len(llm.compactReq.Memories))
	}
	if llm.compactReq.TargetCount != 2 {
		t.Fatalf("expected target count 2, got %d", llm.compactReq.TargetCount)
	}
}

func TestBuiltinProviderCompactDoesNotReplaceWhenLLMFails(t *testing.T) {
	t.Parallel()
	runtime := newFakeCompactRuntime([]adapters.MemoryItem{
		{ID: "bot-1:mem_1", Memory: "one"},
		{ID: "bot-1:mem_2", Memory: "two"},
	})
	llm := &fakeLLM{compactErr: errFakeCompact}
	p := NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	p.SetLLM(llm)

	if _, err := p.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.5, 0); err == nil {
		t.Fatal("expected compact error")
	}
	if runtime.replaceCalls != 0 {
		t.Fatalf("replace should not be called when LLM fails, got %d", runtime.replaceCalls)
	}
}

func TestBuiltinProviderCompactDoesNotReplaceWhenLLMReturnsNoFacts(t *testing.T) {
	t.Parallel()
	runtime := newFakeCompactRuntime([]adapters.MemoryItem{
		{ID: "bot-1:mem_1", Memory: "one"},
		{ID: "bot-1:mem_2", Memory: "two"},
	})
	llm := &fakeLLM{compactFacts: []string{"", "  "}}
	p := NewBuiltinProvider(slog.Default(), runtime, nil, nil)
	p.SetLLM(llm)

	if _, err := p.Compact(context.Background(), map[string]any{"bot_id": "bot-1"}, 0.5, 0); err == nil {
		t.Fatal("expected compact error")
	}
	if runtime.replaceCalls != 0 {
		t.Fatalf("replace should not be called when LLM returns no facts, got %d", runtime.replaceCalls)
	}
}

func TestNewBuiltinRuntimeFromConfig_DefaultReturnsFileRuntime(t *testing.T) {
	t.Parallel()
	sentinel := "file-runtime-sentinel"
	rt, err := NewBuiltinRuntimeFromConfig(nil, nil, sentinel, nil, nil, defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != sentinel {
		t.Fatalf("expected file runtime sentinel, got %v", rt)
	}
}

func TestNewBuiltinRuntimeFromConfig_DenseErrorPropagates(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{"memory_mode": "dense"}
	_, err := NewBuiltinRuntimeFromConfig(nil, cfg, "fallback", nil, nil, defaultTestConfig())
	if err == nil {
		t.Fatal("expected error for dense mode without embedding_model_id")
	}
}

func TestNewBuiltinRuntimeFromConfig_SparseErrorPropagates(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{"memory_mode": "sparse"}
	_, err := NewBuiltinRuntimeFromConfig(nil, cfg, "fallback", nil, nil, defaultTestConfig())
	if err == nil {
		t.Fatal("expected error for sparse mode without encoder base URL")
	}
}

func defaultTestConfig() config.Config {
	return config.Config{}
}

// Fakes from sparse_runtime_test.go are in the same package and accessible.

var _ sparseEncoder = (*fakeSparseEncoder)(nil)

func init() {
	_ = sparse.SparseVector{}
}

type fakeCompactRuntime struct {
	items        []adapters.MemoryItem
	replaced     []adapters.MemoryItem
	replaceCalls int
}

func newFakeCompactRuntime(items []adapters.MemoryItem) *fakeCompactRuntime {
	return &fakeCompactRuntime{items: items}
}

func (r *fakeCompactRuntime) Add(context.Context, adapters.AddRequest) (adapters.SearchResponse, error) {
	return adapters.SearchResponse{}, nil
}

func (r *fakeCompactRuntime) Search(context.Context, adapters.SearchRequest) (adapters.SearchResponse, error) {
	return adapters.SearchResponse{}, nil
}

func (r *fakeCompactRuntime) GetAll(context.Context, adapters.GetAllRequest) (adapters.SearchResponse, error) {
	return adapters.SearchResponse{Results: append([]adapters.MemoryItem(nil), r.items...)}, nil
}

func (r *fakeCompactRuntime) Update(context.Context, adapters.UpdateRequest) (adapters.MemoryItem, error) {
	return adapters.MemoryItem{}, nil
}

func (r *fakeCompactRuntime) Delete(context.Context, string) (adapters.DeleteResponse, error) {
	return adapters.DeleteResponse{}, nil
}

func (r *fakeCompactRuntime) DeleteBatch(context.Context, []string) (adapters.DeleteResponse, error) {
	return adapters.DeleteResponse{}, nil
}

func (r *fakeCompactRuntime) DeleteAll(context.Context, adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	return adapters.DeleteResponse{}, nil
}

func (r *fakeCompactRuntime) ReplaceAll(_ context.Context, _ map[string]any, items []adapters.MemoryItem) error {
	r.replaceCalls++
	r.replaced = append([]adapters.MemoryItem(nil), items...)
	return nil
}

func (r *fakeCompactRuntime) Usage(context.Context, map[string]any) (adapters.UsageResponse, error) {
	return adapters.UsageResponse{}, nil
}

func (r *fakeCompactRuntime) Mode() string { return string(ModeOff) }

func (r *fakeCompactRuntime) Status(context.Context, string) (adapters.MemoryStatusResponse, error) {
	return adapters.MemoryStatusResponse{}, nil
}

func (r *fakeCompactRuntime) Rebuild(context.Context, string) (adapters.RebuildResult, error) {
	return adapters.RebuildResult{}, nil
}
