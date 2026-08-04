package native

import (
	"testing"

	"github.com/google/uuid"

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
