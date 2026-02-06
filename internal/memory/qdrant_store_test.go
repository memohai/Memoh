package memory

import "testing"

func TestBuildQdrantFilter(t *testing.T) {
	t.Parallel()

	filter := buildQdrantFilter(map[string]any{
		"userId": "u1",
		"score":  map[string]any{"gte": 0.5},
	})
	if filter == nil {
		t.Fatalf("expected filter")
	}
	if len(filter.Must) != 2 {
		t.Fatalf("expected two conditions, got %d", len(filter.Must))
	}
}
