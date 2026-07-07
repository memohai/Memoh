package heartbeat

import (
	"context"
	"log/slog"
	"testing"

	memprovider "github.com/memohai/memoh/internal/memory/adapters"
)

func TestNormalizeHeartbeatIntervalDefault(t *testing.T) {
	t.Parallel()

	if got := normalizeHeartbeatInterval(0); got != 1440 {
		t.Fatalf("normalizeHeartbeatInterval(0) = %d, want 1440", got)
	}
	if got := normalizeHeartbeatInterval(-5); got != 1440 {
		t.Fatalf("normalizeHeartbeatInterval(-5) = %d, want 1440", got)
	}
	if got := normalizeHeartbeatInterval(60); got != 60 {
		t.Fatalf("normalizeHeartbeatInterval(60) = %d, want 60", got)
	}
}

// stubIngestProvider is a minimal Provider that also implements
// MarkdownIngestProvider. It embeds memprovider.Provider (zero-value methods
// for the unused surface) and only overrides Type + IngestFromMarkdown.
type stubIngestProvider struct {
	memprovider.Provider
	ingested    memprovider.IngestResult
	err         error
	called      bool
	calledBotID string
}

func (*stubIngestProvider) Type() string { return "builtin" }
func (s *stubIngestProvider) IngestFromMarkdown(_ context.Context, botID string) (memprovider.IngestResult, error) {
	s.called = true
	s.calledBotID = botID
	return s.ingested, s.err
}

func TestIngestMemoryFilesNoRegistry(t *testing.T) {
	t.Parallel()
	svc := &Service{logger: slog.Default()}
	// No memoryRegistry set — should be a silent no-op, not a panic.
	svc.ingestMemoryFiles(t.Context(), "bot-1")
}

func TestIngestMemoryFilesCallsProvider(t *testing.T) {
	t.Parallel()
	registry := memprovider.NewRegistry(slog.Default())
	stub := &stubIngestProvider{
		ingested: memprovider.IngestResult{Ingested: 3, Skipped: 1},
	}
	registry.Register(defaultBuiltinProviderID, stub)

	svc := &Service{
		memoryRegistry: registry,
		logger:         slog.Default(),
	}
	svc.ingestMemoryFiles(t.Context(), "bot-1")

	if !stub.called {
		t.Fatal("expected IngestFromMarkdown to be called")
	}
	if stub.calledBotID != "bot-1" {
		t.Fatalf("IngestFromMarkdown called with botID %q, want %q", stub.calledBotID, "bot-1")
	}
}
