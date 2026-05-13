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
	if !strings.Contains(err.Error(), "[mcp]") || !strings.Contains(err.Error(), "[container]") {
		t.Fatalf("expected migration error mentioning [mcp] and [container], got %v", err)
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
	if !strings.Contains(err.Error(), "both [mcp] and [workspace]") || !strings.Contains(err.Error(), "[container]") {
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

func TestLoadReadsWorkspaceFieldsFromContainerSection(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	data := []byte(`
[container]
backend = "docker"
default_image = "alpine:3.22"
image_pull_policy = "always"
runtime_dir = "/opt/memoh/runtime"
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Container.Backend != "docker" {
		t.Fatalf("container backend = %q", cfg.Container.Backend)
	}
	if cfg.Workspace.DefaultImage != "alpine:3.22" {
		t.Fatalf("workspace default_image = %q", cfg.Workspace.DefaultImage)
	}
	if cfg.Container.DefaultImage != "alpine:3.22" {
		t.Fatalf("container default_image = %q", cfg.Container.DefaultImage)
	}
	if cfg.Workspace.ImagePullPolicy != "always" {
		t.Fatalf("workspace image_pull_policy = %q", cfg.Workspace.ImagePullPolicy)
	}
	if cfg.Workspace.RuntimeDir != "/opt/memoh/runtime" {
		t.Fatalf("workspace runtime_dir = %q", cfg.Workspace.RuntimeDir)
	}
}

func TestLoadRejectsMixedWorkspaceFields(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	data := []byte(`
[container]
backend = "docker"
default_image = "alpine:3.22"

[workspace]
default_image = "debian:bookworm-slim"
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected mixed [container]/[workspace] fields to fail")
	}
	if !strings.Contains(err.Error(), "both [container] and [workspace]") {
		t.Fatalf("expected mixed section error, got %v", err)
	}
}

func TestLoadReadsBackendSpecificConfigs(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	data := []byte(`
[docker]
host = "unix:///var/run/docker.sock"

[apple]
socket_path = "/tmp/socktainer.sock"
binary_path = "/opt/homebrew/bin/socktainer"
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Docker.Host != "unix:///var/run/docker.sock" {
		t.Fatalf("docker host = %q", cfg.Docker.Host)
	}
	if cfg.Apple.SocketPath != "/tmp/socktainer.sock" {
		t.Fatalf("apple socket path = %q", cfg.Apple.SocketPath)
	}
	if cfg.Apple.BinaryPath != "/opt/homebrew/bin/socktainer" {
		t.Fatalf("apple binary path = %q", cfg.Apple.BinaryPath)
	}
}

func TestLoadAppLocalTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "conf", "app.local.toml"))
	if err != nil {
		t.Fatalf("read app.local.toml: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "app.local.toml")
	//nolint:gosec // configPath is rooted at t.TempDir() with a literal filename; the rendered template content is not used as a path.
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("write rendered app.local.toml: %v", err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load app.local.toml: %v", err)
	}
	if cfg.Container.Backend != "docker" {
		t.Fatalf("container backend = %q, want docker", cfg.Container.Backend)
	}
	if !cfg.Local.Enabled {
		t.Fatal("local workspace should be enabled")
	}
	if cfg.Database.DriverOrDefault() != "sqlite" {
		t.Fatalf("database driver = %q, want sqlite", cfg.Database.DriverOrDefault())
	}
	if !filepath.IsAbs(cfg.Workspace.DataRoot) {
		t.Fatalf("workspace data_root = %q, want absolute path", cfg.Workspace.DataRoot)
	}
	if !filepath.IsAbs(cfg.Workspace.RuntimeDir) {
		t.Fatalf("workspace runtime_dir = %q, want absolute path", cfg.Workspace.RuntimeDir)
	}
	if !filepath.IsAbs(cfg.SQLite.Path) {
		t.Fatalf("sqlite path = %q, want absolute path", cfg.SQLite.Path)
	}
	if !filepath.IsAbs(cfg.Registry.ProvidersPath()) {
		t.Fatalf("providers path = %q, want absolute path", cfg.Registry.ProvidersPath())
	}
}

func TestLoadResolvesRelativeLocalPaths(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	data := []byte(`
[database]
driver = "sqlite"

[container]
data_root = "data/local"
runtime_dir = "data/runtime"

[local]
metadata_root = "data/local/containers"

[sqlite]
path = "data/local/memoh.db"

[registry]
providers_dir = "conf/providers"
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for name, path := range map[string]string{
		"data_root":     cfg.Workspace.DataRoot,
		"runtime_dir":   cfg.Workspace.RuntimeDir,
		"metadata_root": cfg.Local.MetadataRoot,
		"sqlite.path":   cfg.SQLite.Path,
		"providers_dir": cfg.Registry.ProvidersPath(),
	} {
		if !filepath.IsAbs(path) {
			t.Fatalf("%s = %q, want absolute path", name, path)
		}
	}
}

func TestWorkspaceImagePullPolicyDefaultsAndNormalizes(t *testing.T) {
	if got := (WorkspaceConfig{}).EffectiveImagePullPolicy(); got != ImagePullPolicyIfNotPresent {
		t.Fatalf("default policy = %q", got)
	}
	if got := (WorkspaceConfig{ImagePullPolicy: "always"}).EffectiveImagePullPolicy(); got != ImagePullPolicyAlways {
		t.Fatalf("always policy = %q", got)
	}
	if got := (WorkspaceConfig{ImagePullPolicy: "invalid"}).EffectiveImagePullPolicy(); got != ImagePullPolicyIfNotPresent {
		t.Fatalf("invalid policy = %q", got)
	}
}

func TestWorkspaceImagePullCandidatesAddsVNCMirror(t *testing.T) {
	got := WorkspaceImagePullCandidates("memohai/vnc:debian")
	want := []string{"docker.io/memohai/vnc:debian", "memoh.cn/memohai/vnc:debian"}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWorkspaceImagePullCandidatesDoesNotMirrorCustomImages(t *testing.T) {
	got := WorkspaceImagePullCandidates("debian:bookworm-slim")
	if len(got) != 1 || got[0] != "docker.io/library/debian:bookworm-slim" {
		t.Fatalf("unexpected candidates: %v", got)
	}
}
