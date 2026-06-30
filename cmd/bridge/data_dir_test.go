package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitDataDirAtMigratesLegacyIdentity(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	templatesDir := t.TempDir()
	legacy := []byte("# IDENTITY.md\n\nCustom legacy identity.\n")
	if err := os.WriteFile(filepath.Join(dataDir, legacyIdentityFileName), legacy, 0o600); err != nil {
		t.Fatalf("write legacy identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, agentsFileName), []byte("# AGENTS.md\n"), 0o600); err != nil {
		t.Fatalf("write agents template: %v", err)
	}

	initDataDirAt(dataDir, templatesDir)

	data, err := os.ReadFile(filepath.Join(dataDir, agentsFileName)) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read migrated agents file: %v", err)
	}
	if string(data) != string(legacy) {
		t.Fatalf("AGENTS.md = %q, want legacy identity content %q", data, legacy)
	}
	if _, err := os.Stat(filepath.Join(dataDir, legacyIdentityFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected legacy identity to be renamed away, got err=%v", err)
	}
}

func TestInitDataDirAtSeedsMissingTemplateFiles(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	templatesDir := t.TempDir()
	agentsTemplate := []byte("# AGENTS.md\n")
	hooksTemplate := []byte("hooks template\n")
	if err := os.WriteFile(filepath.Join(templatesDir, agentsFileName), agentsTemplate, 0o600); err != nil {
		t.Fatalf("write agents template: %v", err)
	}
	writeTestFile(t, templatesDir, ".memoh/hooks.json", hooksTemplate)

	initDataDirAt(dataDir, templatesDir)

	for _, tt := range []struct {
		name string
		rel  string
		want []byte
	}{
		{name: "root template", rel: agentsFileName, want: agentsTemplate},
		{name: "nested template", rel: ".memoh/hooks.json", want: hooksTemplate},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(tt.rel))) //nolint:gosec // test path is under t.TempDir.
			if err != nil {
				t.Fatalf("read seeded %s: %v", tt.rel, err)
			}
			if string(data) != string(tt.want) {
				t.Fatalf("%s = %q, want template content %q", tt.rel, data, tt.want)
			}
		})
	}
}

func TestInitDataDirAtPreservesExistingHooksConfig(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	templatesDir := t.TempDir()
	writeTestFile(t, templatesDir, ".memoh/hooks.json", []byte("template\n"))
	writeTestFile(t, dataDir, ".memoh/hooks.json", []byte("custom\n"))

	initDataDirAt(dataDir, templatesDir)

	data, err := os.ReadFile(filepath.Join(dataDir, ".memoh", "hooks.json")) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read hooks config: %v", err)
	}
	if string(data) != "custom\n" {
		t.Fatalf("hooks.json = %q, want existing config", data)
	}
}

func TestInitDataDirAtSyncsManagedSkillsWithoutDeletingCustomSkills(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	templatesDir := t.TempDir()
	writeTestFile(t, templatesDir, ".memoh/skills/builtin/SKILL.md", []byte("managed v2\n"))
	writeTestFile(t, dataDir, ".memoh/skills/builtin/SKILL.md", []byte("managed v1\n"))
	writeTestFile(t, dataDir, ".memoh/skills/custom/SKILL.md", []byte("custom\n"))

	initDataDirAt(dataDir, templatesDir)

	managed, err := os.ReadFile(filepath.Join(dataDir, ".memoh", "skills", "builtin", "SKILL.md")) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read managed skill: %v", err)
	}
	if string(managed) != "managed v2\n" {
		t.Fatalf("managed skill = %q, want updated template", managed)
	}
	custom, err := os.ReadFile(filepath.Join(dataDir, ".memoh", "skills", "custom", "SKILL.md")) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read custom skill: %v", err)
	}
	if string(custom) != "custom\n" {
		t.Fatalf("custom skill = %q, want preserved", custom)
	}
}

func writeTestFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create parent for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
