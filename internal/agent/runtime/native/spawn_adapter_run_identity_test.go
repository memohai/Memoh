package native

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/sessionmode"
	tools "github.com/memohai/memoh/internal/agent/tool"
)

func TestSpawnRunConfigPreservesAdmittedRunID(t *testing.T) {
	const admittedRunID = "77777777-7777-4777-8777-777777777777"
	cfg := runConfigFromSpawnRunConfig(tools.SpawnRunConfig{RunID: " \t" + admittedRunID + "\n"})

	if cfg.RunID != admittedRunID {
		t.Fatalf("RunID = %q, want admitted RunID %q", cfg.RunID, admittedRunID)
	}
}

func TestSpawnRunConfigMintsRunIDForDirectCaller(t *testing.T) {
	first := runConfigFromSpawnRunConfig(tools.SpawnRunConfig{})
	second := runConfigFromSpawnRunConfig(tools.SpawnRunConfig{RunID: " \t"})

	if _, err := uuid.Parse(first.RunID); err != nil {
		t.Fatalf("first RunID = %q, want minted UUID: %v", first.RunID, err)
	}
	if _, err := uuid.Parse(second.RunID); err != nil {
		t.Fatalf("second RunID = %q, want minted UUID: %v", second.RunID, err)
	}
	if first.RunID == second.RunID {
		t.Fatalf("direct callers received the same RunID %q", first.RunID)
	}
}

func TestSpawnAdapterGenerateWithWatchdogCarriesLifecycleSnapshot(t *testing.T) {
	provider := &atomicMockProvider{
		handler: func(_ int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			return &sdk.GenerateResult{Text: "done", FinishReason: sdk.FinishReasonStop}, nil
		},
	}
	result, err := NewSpawnAdapter(newTestAgent()).GenerateWithWatchdog(
		context.Background(),
		tools.SpawnRunConfig{
			Model: &sdk.Model{
				ID:       "spawn-lifecycle-model",
				Provider: provider,
				Type:     sdk.ModelTypeChat,
			},
			Query:       "do the task",
			SessionType: sessionmode.Subagent,
			Identity: tools.SpawnIdentity{
				BotID:      "bot-1",
				SessionID:  "session-1",
				IsSubagent: true,
			},
		},
		func() {},
	)
	if err != nil {
		t.Fatalf("GenerateWithWatchdog error: %v", err)
	}
	if result == nil || result.ContextLifecycle == nil {
		t.Fatalf("GenerateWithWatchdog result = %#v, want lifecycle snapshot", result)
	}
	if result.ContextLifecycle.Counts.Fragments == 0 || result.ContextLifecycle.Counts.Messages == 0 {
		t.Fatalf("lifecycle counts = %+v, want assembled context", result.ContextLifecycle.Counts)
	}
}

func TestSpawnAdapterGenerateFailureCarriesLifecycleSnapshot(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	provider := &atomicMockProvider{
		handler: func(_ int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			return nil, providerErr
		},
	}
	result, err := NewSpawnAdapter(newTestAgent()).Generate(
		context.Background(),
		tools.SpawnRunConfig{
			Model: &sdk.Model{
				ID:       "spawn-failure-model",
				Provider: provider,
				Type:     sdk.ModelTypeChat,
			},
			Query:       "do the task",
			SessionType: sessionmode.Subagent,
			Identity: tools.SpawnIdentity{
				BotID:      "bot-1",
				SessionID:  "session-1",
				IsSubagent: true,
			},
		},
	)
	if err == nil {
		t.Fatal("Generate error = nil, want provider failure")
	}
	if result == nil || result.ContextLifecycle == nil {
		t.Fatalf("Generate result = %#v, want failure lifecycle snapshot", result)
	}
	if result.ContextLifecycle.Counts.Fragments == 0 || result.ContextLifecycle.Counts.Messages == 0 {
		t.Fatalf("failure lifecycle counts = %+v, want assembled context", result.ContextLifecycle.Counts)
	}
}
