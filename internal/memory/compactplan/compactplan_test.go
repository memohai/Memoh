package compactplan

import (
	"context"
	"fmt"
	"strings"
	"testing"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

type recordingLLM struct {
	reqs []adapters.CompactRequest
}

func (*recordingLLM) Extract(context.Context, adapters.ExtractRequest) (adapters.ExtractResponse, error) {
	return adapters.ExtractResponse{}, nil
}

func (*recordingLLM) Decide(context.Context, adapters.DecideRequest) (adapters.DecideResponse, error) {
	return adapters.DecideResponse{}, nil
}

func (r *recordingLLM) Compact(_ context.Context, req adapters.CompactRequest) (adapters.CompactResponse, error) {
	r.reqs = append(r.reqs, req)
	facts := make([]string, 0, req.TargetCount)
	for i := 0; i < req.TargetCount; i++ {
		facts = append(facts, fmt.Sprintf("summary %02d from call %02d", i, len(r.reqs)))
	}
	return adapters.CompactResponse{Facts: facts}, nil
}

func (*recordingLLM) DetectLanguage(context.Context, string) (string, error) {
	return "", nil
}

func TestBuildPassesBotIDToAllCompactRequests(t *testing.T) {
	t.Parallel()

	items := make([]storefs.MemoryItem, 0, 24)
	for i := 0; i < 24; i++ {
		items = append(items, storefs.MemoryItem{
			ID:     fmt.Sprintf("bot-1:mem_%02d", i),
			Memory: fmt.Sprintf("memory %02d %s", i, strings.Repeat("x", 1600)),
		})
	}
	llm := &recordingLLM{}

	if _, err := Build(context.Background(), Options{
		BotID: "bot-1",
		Items: items,
		Ratio: 0.25,
		LLM:   llm,
	}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(llm.reqs) < 3 {
		t.Fatalf("expected batch and reducer compact requests, got %d", len(llm.reqs))
	}
	for i, req := range llm.reqs {
		if req.BotID != "bot-1" {
			t.Fatalf("compact request %d bot_id = %q, want bot-1", i, req.BotID)
		}
	}
}
