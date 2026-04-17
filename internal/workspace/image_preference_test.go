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

func TestWorkspaceGPUMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		workspaceMetadataKey: map[string]any{
			"keep": "value",
		},
	}

	updated := withWorkspaceGPUPreference(metadata, WorkspaceGPUConfig{
		Devices: []string{" nvidia.com/gpu=0 ", "amd.com/gpu=1", "nvidia.com/gpu=0"},
	})

	gpu, ok := workspaceGPUFromMetadata(updated)
	if !ok {
		t.Fatal("expected gpu preference to be present")
	}
	if got, want := gpu.Devices, []string{"nvidia.com/gpu=0", "amd.com/gpu=1"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected normalized gpu devices %v, got %v", want, got)
	}
	workspace, ok := updated[workspaceMetadataKey].(map[string]any)
	if !ok {
		t.Fatal("expected workspace metadata section")
	}
	if workspace["keep"] != "value" {
		t.Fatalf("expected existing workspace metadata to be preserved, got %#v", workspace)
	}
}

func TestWorkspaceGPUExplicitDisableRemainsPresent(t *testing.T) {
	t.Parallel()

	metadata := withWorkspaceGPUPreference(map[string]any{}, WorkspaceGPUConfig{})

	gpu, ok := workspaceGPUFromMetadata(metadata)
	if !ok {
		t.Fatal("expected gpu preference key to remain present")
	}
	if len(gpu.Devices) != 0 {
		t.Fatalf("expected explicit disable with no devices, got %#v", gpu.Devices)
	}
}

func TestWithoutWorkspaceGPUPreferenceRemovesOnlyGPUKey(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		workspaceMetadataKey: map[string]any{
			workspaceGPUMetadataKey: map[string]any{
				workspaceGPUDevicesKey: []any{"nvidia.com/gpu=all"},
			},
			"keep": true,
		},
	}

	updated := withoutWorkspaceGPUPreference(metadata)
	if _, ok := workspaceGPUFromMetadata(updated); ok {
		t.Fatal("expected gpu preference to be cleared")
	}
	workspace, ok := updated[workspaceMetadataKey].(map[string]any)
	if !ok {
		t.Fatal("expected workspace metadata section to remain")
	}
	if workspace["keep"] != true {
		t.Fatalf("expected unrelated workspace metadata to remain, got %#v", workspace)
	}
}

func TestWorkspaceSkillDiscoveryRootsMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		workspaceMetadataKey: map[string]any{
			"keep": "value",
		},
	}

	updated := withWorkspaceSkillDiscoveryRoots(metadata, []string{
		" /custom/skills ",
		"/root/.openclaw/skills",
		"/custom/skills",
		"/custom/./skills",
		"/data/skills",
		"relative/path",
	})

	roots, ok := workspaceSkillDiscoveryRootsFromMetadata(updated)
	if !ok {
		t.Fatal("expected skill discovery roots preference to be present")
	}
	if got, want := roots, []string{"/custom/skills", "/root/.openclaw/skills"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected normalized skill discovery roots %v, got %v", want, got)
	}
	workspace, ok := updated[workspaceMetadataKey].(map[string]any)
	if !ok {
		t.Fatal("expected workspace metadata section")
	}
	if workspace["keep"] != "value" {
		t.Fatalf("expected existing workspace metadata to be preserved, got %#v", workspace)
	}
}

func TestWorkspaceSkillDiscoveryRootsExplicitDisableRemainsPresent(t *testing.T) {
	t.Parallel()

	metadata := withWorkspaceSkillDiscoveryRoots(map[string]any{}, []string{})

	roots, ok := workspaceSkillDiscoveryRootsFromMetadata(metadata)
	if !ok {
		t.Fatal("expected skill discovery roots key to remain present")
	}
	if roots == nil {
		t.Fatal("expected explicit disable to return a non-nil empty slice")
	}
	if len(roots) != 0 {
		t.Fatalf("expected explicit disable with no roots, got %#v", roots)
	}
}

func TestWithoutWorkspaceSkillDiscoveryRootsRemovesOnlyThatKey(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		workspaceMetadataKey: map[string]any{
			workspaceSkillDiscoveryRootsMetadataKey: []any{"/data/.agents/skills"},
			"keep":                                  true,
		},
	}

	updated := withoutWorkspaceSkillDiscoveryRoots(metadata)
	if _, ok := workspaceSkillDiscoveryRootsFromMetadata(updated); ok {
		t.Fatal("expected skill discovery roots preference to be cleared")
	}
	workspace, ok := updated[workspaceMetadataKey].(map[string]any)
	if !ok {
		t.Fatal("expected workspace metadata section to remain")
	}
	if workspace["keep"] != true {
		t.Fatalf("expected unrelated workspace metadata to remain, got %#v", workspace)
	}
}
