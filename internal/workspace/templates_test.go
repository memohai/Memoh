package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedBridgeTemplatesMigratesLegacyIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacy := []byte("# IDENTITY.md\n\nCustom legacy identity.\n")
	if err := os.WriteFile(filepath.Join(dir, legacyIdentityFileName), legacy, 0o600); err != nil {
		t.Fatalf("write legacy identity: %v", err)
	}

	if err := seedBridgeTemplates(dir); err != nil {
		t.Fatalf("seed bridge templates: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, agentsFileName)) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read migrated agents file: %v", err)
	}
	if string(data) != string(legacy) {
		t.Fatalf("AGENTS.md = %q, want legacy identity content %q", data, legacy)
	}
	if _, err := os.Stat(filepath.Join(dir, legacyIdentityFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected legacy identity to be renamed away, got err=%v", err)
	}
}

func TestSeedBridgeTemplatesCreatesAgentsWhenNoLegacyIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := seedBridgeTemplates(dir); err != nil {
		t.Fatalf("seed bridge templates: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, agentsFileName)) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read seeded agents file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected seeded AGENTS.md to be non-empty")
	}
}
