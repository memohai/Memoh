package agent

import (
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/contextfrag"
)

func TestRefreshContextFragOmitsMaterializedQuery(t *testing.T) {
	t.Parallel()

	cfg := RunConfig{
		System:                   "base system",
		Query:                    "current user",
		Messages:                 []sdk.Message{sdk.UserMessage("current user")},
		ContextQueryMaterialized: true,
	}

	cfg = cfg.RefreshContextFrag()

	if manifestHasAgentKind(cfg.ContextManifest, contextfrag.KindCurrentUserMessage) {
		t.Fatalf("manifest should not include pending current user query after it was materialized: %#v", cfg.ContextManifest.Items)
	}
	if cfg.ContextManifest.Counts.Messages != 1 {
		t.Fatalf("manifest message count = %d, want 1", cfg.ContextManifest.Counts.Messages)
	}
}

func TestRefreshContextFragMarksToolUsageBeforeWorkspaceInstructions(t *testing.T) {
	t.Parallel()

	cfg := RunConfig{
		System: appendToolUsageToSystem(
			"base system\n\n## Workspace instruction files\n\nworkspace text",
			"## Tool usage\n\nusage text",
		),
		ContextToolUsage: "## Tool usage\n\nusage text",
		Messages:         []sdk.Message{sdk.UserMessage("hi")},
	}

	cfg = cfg.RefreshContextFrag()

	toolIndex := manifestAgentKindIndex(cfg.ContextManifest, contextfrag.KindToolUsage)
	workspaceIndex := manifestAgentKindIndex(cfg.ContextManifest, contextfrag.KindWorkspaceInstruction)
	if toolIndex < 0 {
		t.Fatalf("manifest missing tool usage item: %#v", cfg.ContextManifest.Items)
	}
	if workspaceIndex < 0 {
		t.Fatalf("manifest missing workspace instruction item: %#v", cfg.ContextManifest.Items)
	}
	if toolIndex > workspaceIndex {
		t.Fatalf("tool usage manifest index = %d, workspace index = %d; want tool usage before workspace", toolIndex, workspaceIndex)
	}
	rendered := contextfrag.Render(cfg.ContextFrags)
	if rendered.System != cfg.System {
		t.Fatalf("rendered system = %q, want %q", rendered.System, cfg.System)
	}
}

func TestSpawnRunConfigCarriesContextScopeAndMaterializedQuery(t *testing.T) {
	t.Parallel()

	rc := runConfigFromSpawnRunConfig(tools.SpawnRunConfig{
		Query:    "do the task",
		Messages: []sdk.Message{sdk.UserMessage("history")},
		Identity: tools.SpawnIdentity{
			BotID:             "bot-1",
			ChatID:            "chat-1",
			SessionID:         "child-1",
			ChannelIdentityID: "identity-1",
			CurrentPlatform:   "telegram",
			IsSubagent:        true,
		},
	})

	if !rc.ContextQueryMaterialized {
		t.Fatal("spawn query should be marked materialized because it is appended to Messages")
	}
	rc = rc.RefreshContextFrag()
	if manifestHasAgentKind(rc.ContextManifest, contextfrag.KindCurrentUserMessage) {
		t.Fatalf("manifest should not include duplicate pending current user query: %#v", rc.ContextManifest.Items)
	}
	if rc.ContextManifest.Counts.Messages != 2 {
		t.Fatalf("manifest message count = %d, want history + materialized query", rc.ContextManifest.Counts.Messages)
	}
	for _, item := range rc.ContextManifest.Items {
		if item.Scope.BotID != "bot-1" || item.Scope.SessionID != "child-1" || item.Scope.ChannelIdentityID != "identity-1" {
			t.Fatalf("manifest item lost subagent scope: %#v", item.Scope)
		}
	}
}

func TestRefreshContextFragWithDynamicMutatorsMarksPreProviderBoundary(t *testing.T) {
	t.Parallel()

	injectCh := make(chan InjectMessage)
	cfg := RunConfig{
		System:            "base system",
		Messages:          []sdk.Message{sdk.UserMessage("hi")},
		InjectCh:          injectCh,
		BackgroundManager: background.New(nil),
	}
	cfg = cfg.RefreshContextFragWithDynamicMutators(true, true, true)

	if cfg.ContextManifest.View != contextfrag.ViewRunConfigPreProvider {
		t.Fatalf("manifest view = %q, want %q", cfg.ContextManifest.View, contextfrag.ViewRunConfigPreProvider)
	}
	for _, want := range []contextfrag.DynamicMutator{
		contextfrag.DynamicMutatorInjectCh,
		contextfrag.DynamicMutatorReadMedia,
		contextfrag.DynamicMutatorBeforeModelCallHook,
		contextfrag.DynamicMutatorBackgroundSummary,
		contextfrag.DynamicMutatorMidTaskPrune,
	} {
		if !manifestHasMutator(cfg.ContextManifest, want) {
			t.Fatalf("manifest dynamic mutators = %#v, want %q", cfg.ContextManifest.DynamicMutators, want)
		}
	}
}

func manifestHasAgentKind(manifest contextfrag.Manifest, kind contextfrag.Kind) bool {
	return manifestAgentKindIndex(manifest, kind) >= 0
}

func manifestHasMutator(manifest contextfrag.Manifest, want contextfrag.DynamicMutator) bool {
	for _, got := range manifest.DynamicMutators {
		if got == want {
			return true
		}
	}
	return false
}

func manifestAgentKindIndex(manifest contextfrag.Manifest, kind contextfrag.Kind) int {
	for i, item := range manifest.Items {
		if item.Kind == kind {
			return i
		}
	}
	return -1
}
