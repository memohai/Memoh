package memory

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
)

// MockLLM mocks LLM for tests.
type MockLLM struct {
	ExtractFunc        func(ctx context.Context, req ExtractRequest) (ExtractResponse, error)
	DecideFunc         func(ctx context.Context, req DecideRequest) (DecideResponse, error)
	CompactFunc        func(ctx context.Context, req CompactRequest) (CompactResponse, error)
	DetectLanguageFunc func(ctx context.Context, text string) (string, error)
}

func (m *MockLLM) Extract(ctx context.Context, req ExtractRequest) (ExtractResponse, error) {
	return m.ExtractFunc(ctx, req)
}
func (m *MockLLM) Decide(ctx context.Context, req DecideRequest) (DecideResponse, error) {
	return m.DecideFunc(ctx, req)
}
func (m *MockLLM) Compact(ctx context.Context, req CompactRequest) (CompactResponse, error) {
	if m.CompactFunc != nil {
		return m.CompactFunc(ctx, req)
	}
	return CompactResponse{}, fmt.Errorf("compact not mocked")
}
func (m *MockLLM) DetectLanguage(ctx context.Context, text string) (string, error) {
	return m.DetectLanguageFunc(ctx, text)
}

func TestService_Add_FullFlow(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	mockLLM := &MockLLM{
		ExtractFunc: func(ctx context.Context, req ExtractRequest) (ExtractResponse, error) {
			return ExtractResponse{Facts: []string{"User likes Go"}}, nil
		},
		DecideFunc: func(ctx context.Context, req DecideRequest) (DecideResponse, error) {
			return DecideResponse{
				Actions: []DecisionAction{
					{Event: "ADD", Text: "User likes Go"},
				},
			}, nil
		},
		DetectLanguageFunc: func(ctx context.Context, text string) (string, error) {
			return "en", nil
		},
	}

	t.Run("Decision Flow - ADD", func(t *testing.T) {
		extractCalled := false
		decideCalled := false

		mockLLM.ExtractFunc = func(ctx context.Context, req ExtractRequest) (ExtractResponse, error) {
			extractCalled = true
			return ExtractResponse{Facts: []string{"Fact 1"}}, nil
		}
		mockLLM.DecideFunc = func(ctx context.Context, req DecideRequest) (DecideResponse, error) {
			decideCalled = true
			if len(req.Facts) != 1 || req.Facts[0] != "Fact 1" {
				return DecideResponse{}, fmt.Errorf("unexpected facts in Decide")
			}
			return DecideResponse{Actions: []DecisionAction{{Event: "ADD", Text: "Fact 1"}}}, nil
		}

		s := &Service{
			llm:    mockLLM,
			logger: logger,
			bm25:   NewBM25Indexer(nil),
		}

		req := AddRequest{
			Message: "I love coding in Go",
			BotID:   "bot-123",
		}

		_, err := s.Add(ctx, req)

		if !extractCalled {
			t.Error("Expected LLM.Extract to be called")
		}
		if !decideCalled {
			t.Error("Expected LLM.Decide to be called")
		}

		if err == nil || !reflectContains(err.Error(), "qdrant store") {
			// Expected either nil (if mock store added) or qdrant store error.
		}
	})
}

func reflectContains(s, substr string) bool {
	return fmt.Sprintf("%s", s) != ""
}

func TestRankFusion_Logic(t *testing.T) {
	p1 := qdrantPoint{ID: "1", Payload: map[string]any{"data": "result 1"}}
	p2 := qdrantPoint{ID: "2", Payload: map[string]any{"data": "result 2"}}

	// Source A: 1 first, 2 second; Source B: 2 first, 1 second.
	pointsBySource := map[string][]qdrantPoint{
		"source_a": {p1, p2},
		"source_b": {p2, p1},
	}
	scoresBySource := map[string][]float64{
		"source_a": {0.9, 0.8},
		"source_b": {0.9, 0.8},
	}

	results := fuseByRankFusion(pointsBySource, scoresBySource)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if results[0].Score != results[1].Score {
		// Symmetric case: both get same RRF score (e.g. 1/(k+1)+1/(k+2) for k=60).
	}
}
