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
[containerd]
runtime_type = "io.containerd.kata.v2"

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
	if cfg.Containerd.RuntimeType != "io.containerd.kata.v2" {
		t.Fatalf("containerd runtime type = %q", cfg.Containerd.RuntimeType)
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

func TestLoadAppliesBridgeTLSEnvOverrides(t *testing.T) {
	t.Setenv("MEMOH_INSTANCE_ID", "instance-1")
	t.Setenv("MEMOH_BRIDGE_TLS_MODE", BridgeTLSModeStrict)
	t.Setenv("MEMOH_BRIDGE_TLS_SERVER_DIR", "/server")
	t.Setenv("MEMOH_BRIDGE_TLS_BRIDGE_DIR", "/bridge")
	t.Setenv("MEMOH_BRIDGE_TLS_SERVER_NAME", "bridge.internal")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.InstanceID != "instance-1" {
		t.Fatalf("instance id = %q", cfg.InstanceID)
	}
	if cfg.BridgeTLS.Mode != BridgeTLSModeStrict ||
		cfg.BridgeTLS.ServerDir != "/server" ||
		cfg.BridgeTLS.BridgeDir != "/bridge" ||
		cfg.BridgeTLS.ServerName != "bridge.internal" {
		t.Fatalf("bridge tls config = %#v", cfg.BridgeTLS)
	}
}

func TestLoadAppliesPostgresAppEnvOverrides(t *testing.T) {
	t.Setenv("MEMOH_POSTGRES_APP_USER", "memoh_app")
	t.Setenv("MEMOH_POSTGRES_APP_PASSWORD", "env-secret")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Postgres.AppUser != "memoh_app" || cfg.Postgres.AppPassword != "env-secret" {
		t.Fatalf("postgres app credential = %q/%q, want memoh_app/env-secret", cfg.Postgres.AppUser, cfg.Postgres.AppPassword)
	}
}

func TestLoadAppliesWebhookTunnelEnvOverrides(t *testing.T) {
	t.Setenv("MEMOH_WEBHOOK_TUNNEL_MODE", WebhookTunnelModeExternal)
	t.Setenv("MEMOH_WEBHOOK_PUBLIC_BASE_URL", "https://memoh.example.com")
	t.Setenv("MEMOH_WEBHOOK_TUNNEL_LISTEN_ADDR", ":18732")
	t.Setenv("MEMOH_CLOUDFLARED_BIN", "/usr/local/bin/cloudflared")
	t.Setenv("MEMOH_WEBHOOK_TUNNEL_TARGET_URL", "http://127.0.0.1:18732")
	t.Setenv("MEMOH_WEBHOOK_TUNNEL_METRICS_ADDR", "127.0.0.1:18733")
	t.Setenv("MEMOH_WEBHOOK_TUNNEL_METRICS_URL", "http://webhook-tunnel:18733")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.WebhookTunnel.Mode != WebhookTunnelModeExternal ||
		cfg.WebhookTunnel.PublicBaseURL != "https://memoh.example.com" ||
		cfg.WebhookTunnel.ListenAddr != ":18732" ||
		cfg.WebhookTunnel.CloudflaredPath != "/usr/local/bin/cloudflared" ||
		cfg.WebhookTunnel.TargetURL != "http://127.0.0.1:18732" ||
		cfg.WebhookTunnel.MetricsAddr != "127.0.0.1:18733" ||
		cfg.WebhookTunnel.MetricsURL != "http://webhook-tunnel:18733" {
		t.Fatalf("webhook tunnel config = %#v", cfg.WebhookTunnel)
	}
}

func TestLoadRejectsInvalidWebhookTunnelMode(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[webhook_tunnel]\nmode = \"managd\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected invalid webhook tunnel mode to fail")
	}
	if !strings.Contains(err.Error(), "unsupported webhook_tunnel mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidWebhookTunnelModeFromEnv(t *testing.T) {
	t.Setenv("MEMOH_WEBHOOK_TUNNEL_MODE", "managd")

	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil {
		t.Fatal("expected invalid webhook tunnel mode env to fail")
	}
	if !strings.Contains(err.Error(), "unsupported webhook_tunnel mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDefaultsContainerdRuntimeType(t *testing.T) {
	t.Parallel()

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Containerd.RuntimeType != DefaultContainerdRuntimeType {
		t.Fatalf("containerd runtime type = %q, want %q", cfg.Containerd.RuntimeType, DefaultContainerdRuntimeType)
	}
	if got := cfg.Containerd.RuntimeTypeOrDefault(); got != DefaultContainerdRuntimeType {
		t.Fatalf("runtime type default = %q, want %q", got, DefaultContainerdRuntimeType)
	}
}

func TestLoadAppExampleTemplateKeepsRootKeysOutsideAgentSection(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "conf", "app.example.toml"))
	if err != nil {
		t.Fatalf("read app.example.toml: %v", err)
	}
	rendered := strings.Replace(string(raw), `timezone = "UTC"`, `timezone = "Asia/Tokyo"`, 1)
	rendered = strings.Replace(rendered, `instance_id = ""`, `instance_id = "instance-example"`, 1)
	configPath := filepath.Join(t.TempDir(), "app.example.toml")
	if err := os.WriteFile(configPath, []byte(rendered), 0o600); err != nil {
		t.Fatalf("write rendered app.example.toml: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load app.example.toml: %v", err)
	}
	if cfg.Timezone != "Asia/Tokyo" {
		t.Fatalf("timezone = %q, want Asia/Tokyo", cfg.Timezone)
	}
	if cfg.InstanceID != "instance-example" {
		t.Fatalf("instance_id = %q, want instance-example", cfg.InstanceID)
	}
	if cfg.Agent.ToolOutputMaxBytes != DefaultAgentToolOutputBytes ||
		cfg.Agent.ToolOutputMaxLines != DefaultAgentToolOutputLines ||
		cfg.Agent.SystemFilesMaxBytes != DefaultAgentSystemFilesBytes {
		t.Fatalf("agent limits = %#v", cfg.Agent)
	}
}

func TestLoadAppKataDevTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "devenv", "app.kata.dev.toml"))
	if err != nil {
		t.Fatalf("read app.kata.dev.toml: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "app.kata.dev.toml")
	//nolint:gosec // configPath is rooted at t.TempDir() with a literal filename; the rendered template content is not used as a path.
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("write rendered app.kata.dev.toml: %v", err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load app.kata.dev.toml: %v", err)
	}
	if cfg.Container.Backend != "containerd" {
		t.Fatalf("container backend = %q, want containerd", cfg.Container.Backend)
	}
	if cfg.Containerd.RuntimeTypeOrDefault() != "io.containerd.kata.v2" {
		t.Fatalf("containerd runtime type = %q, want io.containerd.kata.v2", cfg.Containerd.RuntimeTypeOrDefault())
	}
	if cfg.Database.DriverOrDefault() != "postgres" {
		t.Fatalf("database driver = %q, want postgres", cfg.Database.DriverOrDefault())
	}
	if !filepath.IsAbs(cfg.Workspace.DataRoot) {
		t.Fatalf("workspace data_root = %q, want absolute path", cfg.Workspace.DataRoot)
	}
	if !filepath.IsAbs(cfg.Workspace.RuntimeDir) {
		t.Fatalf("workspace runtime_dir = %q, want absolute path", cfg.Workspace.RuntimeDir)
	}
}

func TestLoadAppKataDockerTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "conf", "app.kata.docker.toml"))
	if err != nil {
		t.Fatalf("read app.kata.docker.toml: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "app.kata.docker.toml")
	//nolint:gosec // configPath is rooted at t.TempDir() with a literal filename; the rendered template content is not used as a path.
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("write rendered app.kata.docker.toml: %v", err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load app.kata.docker.toml: %v", err)
	}
	if cfg.Container.Backend != "containerd" {
		t.Fatalf("container backend = %q, want containerd", cfg.Container.Backend)
	}
	if cfg.Containerd.RuntimeTypeOrDefault() != "io.containerd.kata.v2" {
		t.Fatalf("containerd runtime type = %q, want io.containerd.kata.v2", cfg.Containerd.RuntimeTypeOrDefault())
	}
	if cfg.Database.DriverOrDefault() != "postgres" {
		t.Fatalf("database driver = %q, want postgres", cfg.Database.DriverOrDefault())
	}
	if !filepath.IsAbs(cfg.Workspace.DataRoot) {
		t.Fatalf("workspace data_root = %q, want absolute path", cfg.Workspace.DataRoot)
	}
	if !filepath.IsAbs(cfg.Workspace.RuntimeDir) {
		t.Fatalf("workspace runtime_dir = %q, want absolute path", cfg.Workspace.RuntimeDir)
	}
}

func TestLoadResolvesRelativeLocalPaths(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	data := []byte(`
[database]
driver = "postgres"

[container]
data_root = "data/local"
runtime_dir = "data/runtime"

[local]
metadata_root = "data/local/containers"

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

func TestWorkspaceImageRefDefaultsToPackagedWorkspace(t *testing.T) {
	got := (WorkspaceConfig{}).ImageRef()
	want := "docker.io/memohai/workspace:debian"
	if got != want {
		t.Fatalf("default image ref = %q, want %q", got, want)
	}
}

func TestWorkspaceImagePullCandidatesAddsWorkspaceMirror(t *testing.T) {
	got := WorkspaceImagePullCandidates("memohai/workspace:debian")
	want := []string{"docker.io/memohai/workspace:debian", "memoh.cn/memohai/workspace:debian"}
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
