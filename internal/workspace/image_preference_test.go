package workspace

import "testing"

func TestWorkspaceBasicMetadataPreferencesRoundTrip(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		"name": "test",
		workspaceMetadataKey: map[string]any{
			"keep": "value",
		},
	}

	updated := withWorkspaceImagePreference(metadata, " alpine:3.20 ")
	updated = withWorkspaceBackendPreference(updated, " local ", " /Users/example/.memoh/workspaces/bot ")

	if got := workspaceImageFromMetadata(updated); got != "alpine:3.20" {
		t.Fatalf("image preference = %q, want alpine:3.20", got)
	}
	if got := workspaceBackendFromMetadata(updated); got != "local" {
		t.Fatalf("workspace backend = %q, want local", got)
	}
	if got := localWorkspacePathFromMetadata(updated); got != "/Users/example/.memoh/workspaces/bot" {
		t.Fatalf("local workspace path = %q", got)
	}
	assertWorkspaceMetadataKeepsValue(t, updated)
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
	assertStringSlice(t, gpu.Devices, []string{"nvidia.com/gpu=0", "amd.com/gpu=1"})
	assertWorkspaceMetadataKeepsValue(t, updated)
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
		"/data/.memoh/skills",
		"relative/path",
	})

	roots, ok := workspaceSkillDiscoveryRootsFromMetadata(updated)
	if !ok {
		t.Fatal("expected skill discovery roots preference to be present")
	}
	assertStringSlice(t, roots, []string{"/custom/skills", "/root/.openclaw/skills"})
	assertWorkspaceMetadataKeepsValue(t, updated)
}

func TestWorkspaceExplicitDisablePreferencesRemainPresent(t *testing.T) {
	t.Parallel()

	t.Run("gpu", func(t *testing.T) {
		metadata := withWorkspaceGPUPreference(map[string]any{}, WorkspaceGPUConfig{})

		gpu, ok := workspaceGPUFromMetadata(metadata)
		if !ok {
			t.Fatal("expected gpu preference key to remain present")
		}
		if len(gpu.Devices) != 0 {
			t.Fatalf("expected explicit disable with no devices, got %#v", gpu.Devices)
		}
	})

	t.Run("skill_discovery_roots", func(t *testing.T) {
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
	})
}

func TestWithoutWorkspacePreferencesRemovesOnlySelectedKey(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		metadata   map[string]any
		clear      func(map[string]any) map[string]any
		isPresent  func(map[string]any) bool
		wantAbsent string
	}{
		{
			name: "image",
			metadata: map[string]any{
				workspaceMetadataKey: map[string]any{
					workspaceImageMetadataKey: "debian:bookworm-slim",
					"keep":                    true,
				},
			},
			clear:      withoutWorkspaceImagePreference,
			isPresent:  func(metadata map[string]any) bool { return workspaceImageFromMetadata(metadata) != "" },
			wantAbsent: "image preference",
		},
		{
			name: "gpu",
			metadata: map[string]any{
				workspaceMetadataKey: map[string]any{
					workspaceGPUMetadataKey: map[string]any{
						workspaceGPUDevicesKey: []any{"nvidia.com/gpu=all"},
					},
					"keep": true,
				},
			},
			clear: withoutWorkspaceGPUPreference,
			isPresent: func(metadata map[string]any) bool {
				_, ok := workspaceGPUFromMetadata(metadata)
				return ok
			},
			wantAbsent: "gpu preference",
		},
		{
			name: "skill_discovery_roots",
			metadata: map[string]any{
				workspaceMetadataKey: map[string]any{
					workspaceSkillDiscoveryRootsMetadataKey: []any{"/data/.agents/skills"},
					"keep":                                  true,
				},
			},
			clear: withoutWorkspaceSkillDiscoveryRoots,
			isPresent: func(metadata map[string]any) bool {
				_, ok := workspaceSkillDiscoveryRootsFromMetadata(metadata)
				return ok
			},
			wantAbsent: "skill discovery roots preference",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updated := tc.clear(tc.metadata)
			if tc.isPresent(updated) {
				t.Fatalf("expected %s to be cleared", tc.wantAbsent)
			}
			if got := workspaceKeepValue(updated); got != true {
				t.Fatalf("expected unrelated workspace metadata to remain, got %#v", got)
			}
		})
	}
}

func assertWorkspaceMetadataKeepsValue(t *testing.T, metadata map[string]any) {
	t.Helper()
	if got := workspaceKeepValue(metadata); got != "value" {
		t.Fatalf("expected existing workspace metadata to be preserved, got %#v", got)
	}
}

func workspaceKeepValue(metadata map[string]any) any {
	workspace, ok := metadata[workspaceMetadataKey].(map[string]any)
	if !ok {
		return nil
	}
	return workspace["keep"]
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
