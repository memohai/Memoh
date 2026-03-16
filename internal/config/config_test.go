package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsLegacyMCPSection(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[mcp]\nfoo = \"legacy\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected load to fail for legacy [mcp] section")
	}
	if !strings.Contains(err.Error(), "[mcp]") || !strings.Contains(err.Error(), "[workspace]") {
		t.Fatalf("expected migration error mentioning [mcp] and [workspace], got %v", err)
	}
}

func TestLoadRejectsMixedMCPAndWorkspaceSections(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[mcp]\nfoo = \"legacy\"\n[workspace]\ndefault_image = \"current\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected load to fail when both [mcp] and [workspace] are present")
	}
	if !strings.Contains(err.Error(), "both [mcp] and [workspace]") {
		t.Fatalf("expected mixed-section error, got %v", err)
	}
}

func TestLoadReadsWorkspaceDefaultImage(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[workspace]\ndefault_image = \"alpine:3.22\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Workspace.DefaultImage != "alpine:3.22" {
		t.Fatalf("expected default_image to load, got %q", cfg.Workspace.DefaultImage)
	}
}
