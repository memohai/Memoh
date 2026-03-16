package workspace

import "testing"

func TestWorkspaceImageMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		"name": "test",
		workspaceMetadataKey: map[string]any{
			"keep": "value",
		},
	}

	updated := withWorkspaceImagePreference(metadata, "alpine:3.20")

	if got := workspaceImageFromMetadata(updated); got != "alpine:3.20" {
		t.Fatalf("expected image preference to round-trip, got %q", got)
	}
	workspace, ok := updated[workspaceMetadataKey].(map[string]any)
	if !ok {
		t.Fatal("expected workspace metadata section")
	}
	if workspace["keep"] != "value" {
		t.Fatalf("expected existing workspace metadata to be preserved, got %#v", workspace)
	}
	if _, exists := metadata[workspaceMetadataKey].(map[string]any)[workspaceImageMetadataKey]; exists {
		t.Fatal("expected original metadata map to remain unchanged")
	}
}

func TestWithoutWorkspaceImagePreferenceRemovesOnlyImageKey(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		workspaceMetadataKey: map[string]any{
			workspaceImageMetadataKey: "debian:bookworm-slim",
			"keep":                    true,
		},
	}

	updated := withoutWorkspaceImagePreference(metadata)
	if got := workspaceImageFromMetadata(updated); got != "" {
		t.Fatalf("expected image preference to be cleared, got %q", got)
	}
	workspace, ok := updated[workspaceMetadataKey].(map[string]any)
	if !ok {
		t.Fatal("expected workspace metadata section to remain")
	}
	if workspace["keep"] != true {
		t.Fatalf("expected unrelated workspace metadata to remain, got %#v", workspace)
	}
}
