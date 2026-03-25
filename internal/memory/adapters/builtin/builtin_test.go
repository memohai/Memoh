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
