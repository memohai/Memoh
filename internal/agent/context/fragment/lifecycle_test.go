package contextfrag_test

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	contextfrag "github.com/memohai/memoh/internal/agent/context/fragment"
	"github.com/memohai/memoh/internal/agent/runtime/native"
)

func TestLifecycleHolderSnapshotIsContentLight(t *testing.T) {
	t.Parallel()

	holder := contextfrag.NewLifecycleHolder()
	if _, ok := holder.Snapshot(); ok {
		t.Fatal("empty holder unexpectedly exposed a snapshot")
	}
	holder.SetManifest(contextfrag.Manifest{
		View: contextfrag.ViewRunConfigPreProvider,
		Counts: contextfrag.ManifestCounts{
			Fragments: 4,
			Messages:  2,
			Images:    1,
			TextBytes: 512,
		},
		Items: []contextfrag.ManifestItem{{ID: "private-content-marker"}},
	})
	holder.SetAssistantMessageID(" assistant-message-1 ")

	snapshot, ok := holder.Snapshot()
	if !ok {
		t.Fatal("expected snapshot after SetManifest")
	}
	if snapshot.Version != 1 || snapshot.View != contextfrag.ViewRunConfigPreProvider {
		t.Fatalf("snapshot identity = (%d, %q), want (1, %q)", snapshot.Version, snapshot.View, contextfrag.ViewRunConfigPreProvider)
	}
	if snapshot.Counts != (contextfrag.ManifestCounts{Fragments: 4, Messages: 2, Images: 1, TextBytes: 512}) {
		t.Fatalf("snapshot counts = %#v", snapshot.Counts)
	}
	if snapshot.AssistantMessageID != "assistant-message-1" {
		t.Fatalf("assistant message ID = %q, want trimmed association", snapshot.AssistantMessageID)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(raw), "private-content-marker") || strings.Contains(string(raw), `"items"`) {
		t.Fatalf("content-light snapshot leaked manifest items: %s", raw)
	}
}

func TestRefreshContextFragUpdatesLifecycleHolder(t *testing.T) {
	t.Parallel()

	holder := contextfrag.NewLifecycleHolder()
	cfg := native.RunConfig{
		System:           "system prompt",
		Messages:         []sdk.Message{sdk.UserMessage("hello")},
		ContextLifecycle: holder,
	}.RefreshContextFrag()

	snapshot, ok := holder.Snapshot()
	if !ok {
		t.Fatal("RefreshContextFrag did not update the lifecycle holder")
	}
	if snapshot.View != cfg.ContextManifest.View || snapshot.Counts != cfg.ContextManifest.Counts {
		t.Fatalf("snapshot = %#v, manifest = %#v", snapshot, cfg.ContextManifest)
	}
}
