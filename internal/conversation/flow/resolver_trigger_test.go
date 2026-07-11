package flow

import (
	"context"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/conversation"
)

func TestFinishPromptCompactionConsumesKnownPressure(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation:      contextbudget.Allocation{CompactableTokens: 31},
	}, nil)
	resolved := resolvedContext{promptState: state}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}

	resolver.finishPromptCompaction(context.Background(), conversation.ChatRequest{}, resolved)

	if _, _, claimed := resolved.claimCompactionPressure(); claimed {
		t.Fatal("finishPromptCompaction() left the receipt unconsumed")
	}
}

func TestFinishPromptCompactionConsumesUnknownPressureWithoutTriggering(t *testing.T) {
	t.Parallel()

	resolved := resolvedContext{promptState: &initialPromptState{}}
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}

	resolver.finishPromptCompaction(context.Background(), conversation.ChatRequest{}, resolved)

	if _, _, claimed := resolved.claimCompactionPressure(); claimed {
		t.Fatal("finishPromptCompaction() left an unknown receipt unconsumed")
	}
}

func TestResolvedContextWithoutReceiptCannotClaimCompaction(t *testing.T) {
	t.Parallel()

	if pressure, known, claimed := (resolvedContext{}).claimCompactionPressure(); claimed || known || pressure != 0 {
		t.Fatalf("claimCompactionPressure() = %d/%v/%v, want 0/false/false without receipt", pressure, known, claimed)
	}
}
