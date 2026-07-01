package edge

import (
	"log/slog"
	"testing"
)

func TestEdgeAdapter_ResolveModel(t *testing.T) {
	t.Parallel()
	adapter := NewEdgeAdapter(slog.Default())

	got, err := adapter.ResolveModel("")
	if err != nil {
		t.Fatalf("ResolveModel default: %v", err)
	}
	if got != edgeModelReadAloud {
		t.Fatalf("ResolveModel default got %q, want %q", got, edgeModelReadAloud)
	}

	got, err = adapter.ResolveModel("EDGE-READ-ALOUD")
	if err != nil {
		t.Fatalf("ResolveModel case-insensitive: %v", err)
	}
	if got != edgeModelReadAloud {
		t.Fatalf("ResolveModel normalized got %q, want %q", got, edgeModelReadAloud)
	}

	if _, err := adapter.ResolveModel("unsupported"); err == nil {
		t.Fatal("expected unsupported model error")
	}
}
