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

func TestInitDataDirAtCreatesAgentsWhenNoLegacyIdentity(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	templatesDir := t.TempDir()
	template := []byte("# AGENTS.md\n")
	if err := os.WriteFile(filepath.Join(templatesDir, agentsFileName), template, 0o600); err != nil {
		t.Fatalf("write agents template: %v", err)
	}

	initDataDirAt(dataDir, templatesDir)

	data, err := os.ReadFile(filepath.Join(dataDir, agentsFileName)) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read seeded agents file: %v", err)
	}
	if string(data) != string(template) {
		t.Fatalf("AGENTS.md = %q, want template content %q", data, template)
	}
}
