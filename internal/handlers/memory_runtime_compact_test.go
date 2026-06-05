package handlers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

type fakeCompactLLM struct {
	facts []string
	fn    func(memprovider.CompactRequest) memprovider.CompactResponse
	calls int
	reqs  []memprovider.CompactRequest
}

func (*fakeCompactLLM) Extract(context.Context, memprovider.ExtractRequest) (memprovider.ExtractResponse, error) {
	return memprovider.ExtractResponse{}, nil
}

func (*fakeCompactLLM) Decide(context.Context, memprovider.DecideRequest) (memprovider.DecideResponse, error) {
	return memprovider.DecideResponse{}, nil
}

func (f *fakeCompactLLM) Compact(_ context.Context, req memprovider.CompactRequest) (memprovider.CompactResponse, error) {
	f.calls++
	f.reqs = append(f.reqs, req)
	if f.fn != nil {
		return f.fn(req), nil
	}
	return memprovider.CompactResponse{Facts: f.facts}, nil
}

func (*fakeCompactLLM) DetectLanguage(context.Context, string) (string, error) {
	return "", nil
}

func TestCompactFileRuntimeItemsWithLLMPreservesProtectedMemories(t *testing.T) {
	t.Parallel()
	llm := &fakeCompactLLM{facts: []string{"Ran likes tea."}}

	active, archived, err := compactFileRuntimeItemsWithLLM(context.Background(), "bot-1", []storefs.MemoryItem{
		{ID: "bot-1:mem_1", Memory: "Pinned preference", Metadata: map[string]any{"pinned": true}},
		{ID: "bot-1:mem_2", Memory: "Read-only profile", Metadata: map[string]any{"read_only": "true"}},
		{ID: "bot-1:mem_3", Memory: "Ran likes green tea"},
		{ID: "bot-1:mem_4", Memory: "Ran likes oolong tea"},
	}, 0.5, 0, llm)
	if err != nil {
		t.Fatalf("compactFileRuntimeItemsWithLLM() error = %v", err)
	}
	if len(llm.reqs) != 1 || len(llm.reqs[0].Memories) != 2 {
		t.Fatalf("expected only compactable memories sent to LLM, got %#v", llm.reqs)
	}
	if len(active) != 3 {
		t.Fatalf("expected 2 protected + 1 compacted active memories, got %d", len(active))
	}
	if len(archived) != 2 {
		t.Fatalf("expected 2 compacted source memories archived, got %d", len(archived))
	}
}

func TestCompactFileRuntimeItemsWithLLMBatchesOversizedInputs(t *testing.T) {
	t.Parallel()
	items := make([]storefs.MemoryItem, 0, 24)
	for i := 0; i < 24; i++ {
		items = append(items, storefs.MemoryItem{
			ID:     fmt.Sprintf("bot-1:mem_%02d", i),
			Memory: fmt.Sprintf("memory %02d %s", i, strings.Repeat("x", 1600)),
		})
	}
	llm := &fakeCompactLLM{}
	llm.fn = func(_ memprovider.CompactRequest) memprovider.CompactResponse {
		return memprovider.CompactResponse{Facts: []string{
			fmt.Sprintf("summary call %02d a", llm.calls),
			fmt.Sprintf("summary call %02d b", llm.calls),
			fmt.Sprintf("summary call %02d c", llm.calls),
		}}
	}

	active, archived, err := compactFileRuntimeItemsWithLLM(context.Background(), "bot-1", items, 0.25, 0, llm)
	if err != nil {
		t.Fatalf("compactFileRuntimeItemsWithLLM() error = %v", err)
	}
	if llm.calls < 2 {
		t.Fatalf("expected oversized input to be compacted in batches, got %d call(s)", llm.calls)
	}
	if len(active) > 6 {
		t.Fatalf("expected final compacted count <= 6, got %d", len(active))
	}
	if len(archived) != 24 {
		t.Fatalf("expected all source memories archived, got %d", len(archived))
	}
}
