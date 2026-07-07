package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	DefaultConfigPath            = "config.toml"
	DefaultHTTPAddr              = ":8080"
	DefaultNamespace             = "default"
	DefaultSocketPath            = "/run/containerd/containerd.sock"
	DefaultDataRoot              = "data"
	DefaultDataMount             = "/data"
	DefaultCNIBinaryDir          = "/opt/cni/bin"
	DefaultCNIConfigDir          = "/etc/cni/net.d"
	DefaultJWTExpiresIn          = "24h"
	DefaultDatabaseDriver        = "postgres"
	DefaultPGHost                = "127.0.0.1"
	DefaultPGPort                = 5432
	DefaultPGUser                = "postgres"
	DefaultPGDatabase            = "memoh"
	DefaultPGSSLMode             = "disable"
	DefaultQdrantURL             = "http://127.0.0.1:6334"
	DefaultQdrantCollection      = "memory"
	DefaultRuntimeDir            = "/opt/memoh/runtime"
	DefaultWorkspaceImage        = "memohai/workspace:debian"
	DefaultBaseImage             = DefaultWorkspaceImage
	DefaultWorkspaceMirrorImage  = "memoh.cn/memohai/workspace:debian"
	DefaultTimezone              = "UTC"
	DefaultContainerdRuntimeType = "io.containerd.runc.v2"
	DefaultAgentToolOutputBytes  = 64 * 1024
	DefaultAgentToolOutputLines  = 2000
	DefaultAgentSystemFilesBytes = 32 * 1024

	ImagePullPolicyIfNotPresent = "if_not_present"
	ImagePullPolicyAlways       = "always"
	ImagePullPolicyNever        = "never"
)

type Config struct {
	Log           LogConfig           `toml:"log"`
	Server        ServerConfig        `toml:"server"`
	Admin         AdminConfig         `toml:"admin"`
	Auth          AuthConfig          `toml:"auth"`
	Agent         AgentConfig         `toml:"agent"`
	Timezone      string              `toml:"timezone"`
	Database      DatabaseConfig      `toml:"database"`
	Container     ContainerConfig     `toml:"container"`
	Containerd    ContainerdConfig    `toml:"containerd"`
	Docker        DockerConfig        `toml:"docker"`
	Apple         AppleConfig         `toml:"apple"`
	Local         LocalConfig         `toml:"local"`
	Workspace     WorkspaceConfig     `toml:"workspace"`
	Postgres      PostgresConfig      `toml:"postgres"`
	Qdrant        QdrantConfig        `toml:"qdrant"`
	Sparse        SparseConfig        `toml:"sparse"`
	Registry      RegistryConfig      `toml:"registry"`
	Supermarket   SupermarketConfig   `toml:"supermarket"`
	OAuthClients  OAuthClientsConfig  `toml:"oauth_clients"`
	InstanceID    string              `toml:"instance_id"`
	BridgeTLS     BridgeTLSConfig     `toml:"bridge_tls"`
	WebhookTunnel WebhookTunnelConfig `toml:"webhook_tunnel"`
}

const (
	BridgeTLSModeDisabled = "disabled"
	BridgeTLSModeStrict   = "strict"
)

// BridgeTLSConfig controls mTLS for Memoh server -> workspace bridge TCP gRPC.
// Strict mode never falls back to plaintext. UDS/local targets are unaffected.
type BridgeTLSConfig struct {
	Mode       string `toml:"mode"`
	ServerDir  string `toml:"server_dir"`
	BridgeDir  string `toml:"bridge_dir"`
	ServerName string `toml:"server_name"`
}

func (c BridgeTLSConfig) EffectiveMode() string {
	mode := strings.TrimSpace(strings.ToLower(c.Mode))
	if mode == "" {
		return BridgeTLSModeDisabled
	}
	return mode
}

func (c BridgeTLSConfig) Strict() bool {
	return c.EffectiveMode() == BridgeTLSModeStrict
}

func BridgeServerName(instanceID string) string {
	return fmt.Sprintf("memoh-bridge.%s.bridge.memoh.internal", instanceID)
}

func BridgeServerSPIFFE(instanceID string) string {
	return fmt.Sprintf("spiffe://memoh/instance/%s/bridge", instanceID)
}

func ServerClientSPIFFE(instanceID string) string {
	return fmt.Sprintf("spiffe://memoh/instance/%s/server", instanceID)
}

type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

type ServerConfig struct {
	Addr string `toml:"addr"`
}

const (
	WebhookTunnelModeDisabled = "disabled"
	WebhookTunnelModeManaged  = "managed"
	WebhookTunnelModeExternal = "external"
)

type WebhookTunnelConfig struct {
	Mode            string `toml:"mode"`
	PublicBaseURL   string `toml:"public_base_url"`
	ListenAddr      string `toml:"listen_addr"`
	CloudflaredPath string `toml:"cloudflared_path"`
	TargetURL       string `toml:"target_url"`
	MetricsAddr     string `toml:"metrics_addr"`
	MetricsURL      string `toml:"metrics_url"`
}

func (c WebhookTunnelConfig) EffectiveMode() string {
	mode := strings.TrimSpace(strings.ToLower(c.Mode))
	if mode == "" {
		return WebhookTunnelModeDisabled
	}
	return mode
}

func (c WebhookTunnelConfig) Validate() error {
	switch c.EffectiveMode() {
	case WebhookTunnelModeDisabled, WebhookTunnelModeManaged, WebhookTunnelModeExternal:
		return nil
	default:
		return fmt.Errorf("unsupported webhook_tunnel mode %q", c.Mode)
	}
}

type AdminConfig struct {
	Username string `toml:"username"`
	Password string `toml:"password" json:"-"`
	Email    string `toml:"email"`
}

type AuthConfig struct {
	JWTSecret    string `toml:"jwt_secret"    json:"-"`
	JWTExpiresIn string `toml:"jwt_expires_in"`
}

type AgentConfig struct {
	ToolOutputMaxBytes  int `toml:"tool_output_max_bytes"`
	ToolOutputMaxLines  int `toml:"tool_output_max_lines"`
	SystemFilesMaxBytes int `toml:"system_files_max_bytes"`
}

type DatabaseConfig struct {
	Driver string `toml:"driver"`
}

func (c DatabaseConfig) DriverOrDefault() string {
	driver := strings.TrimSpace(strings.ToLower(c.Driver))
	if driver == "" {
		return DefaultDatabaseDriver
	}
	return driver
}

type ContainerConfig struct {
	Backend string `toml:"backend"`
	WorkspaceConfig
}

type ContainerdConfig struct {
	SocketPath  string `toml:"socket_path"`
	Namespace   string `toml:"namespace"`
	RuntimeType string `toml:"runtime_type"`
}

func (c ContainerdConfig) RuntimeTypeOrDefault() string {
	runtimeType := strings.TrimSpace(c.RuntimeType)
	if runtimeType == "" {
		return DefaultContainerdRuntimeType
	}
	return runtimeType
}

type DockerConfig struct {
	Host string `toml:"host"`
}

type AppleConfig struct {
	SocketPath string `toml:"socket_path"`
	BinaryPath string `toml:"binary_path"`
}

type LocalConfig struct {
	Enabled                bool   `toml:"enabled"`
	DefaultWorkspaceParent string `toml:"default_workspace_parent"`
	MetadataRoot           string `toml:"metadata_root"`
	AllowAbsolutePaths     bool   `toml:"allow_absolute_paths"`
}

func (c LocalConfig) WorkspaceParent() string {
	if strings.TrimSpace(c.DefaultWorkspaceParent) != "" {
		return absPath(expandHome(strings.TrimSpace(c.DefaultWorkspaceParent)))
	}
	return absPath(filepath.Join(homeDirOrDot(), ".memoh", "workspaces"))
}

func (c LocalConfig) MetadataPath(dataRoot string) string {
	if strings.TrimSpace(c.MetadataRoot) != "" {
		return absPath(expandHome(strings.TrimSpace(c.MetadataRoot)))
	}
	root := strings.TrimSpace(dataRoot)
	if root == "" {
		root = DefaultDataRoot
	}
	return filepath.Join(absPath(root), "local", "containers")
}

type WorkspaceConfig struct {
	Registry        string `toml:"registry"`
	DefaultImage    string `toml:"default_image"`
	ImagePullPolicy string `toml:"image_pull_policy"`
	Snapshotter     string `toml:"snapshotter"`
	DataRoot        string `toml:"data_root"`
	CNIBinaryDir    string `toml:"cni_bin_dir"`
	CNIConfigDir    string `toml:"cni_conf_dir"`
	RuntimeDir      string `toml:"runtime_dir"`
}

// ImageRef returns the fully qualified image reference for the base image,
// prepending the registry mirror when configured and normalizing for containerd
// compatibility.
func (c WorkspaceConfig) ImageRef() string {
	img := c.DefaultImage
	if img == "" {
		img = DefaultBaseImage
	}
	if c.Registry != "" {
		return c.Registry + "/" + img
	}
	return NormalizeImageRef(img)
}

// RuntimePath returns the path to the workspace runtime directory.
func (c WorkspaceConfig) RuntimePath() string {
	if c.RuntimeDir != "" {
		return absPath(c.RuntimeDir)
	}
	return DefaultRuntimeDir
}

func (c WorkspaceConfig) DataRootPath() string {
	if strings.TrimSpace(c.DataRoot) != "" {
		return absPath(c.DataRoot)
	}
	return absPath(DefaultDataRoot)
}

func (c WorkspaceConfig) EffectiveImagePullPolicy() string {
	switch strings.TrimSpace(strings.ToLower(c.ImagePullPolicy)) {
	case ImagePullPolicyAlways:
		return ImagePullPolicyAlways
	case ImagePullPolicyNever:
		return ImagePullPolicyNever
	case ImagePullPolicyIfNotPresent, "":
		return ImagePullPolicyIfNotPresent
	default:
		return ImagePullPolicyIfNotPresent
	}
}

func expandHome(path string) string {
	if path == "~" {
		return homeDirOrDot()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDirOrDot(), path[2:])
	}
	return path
}

func absPath(path string) string {
	path = strings.TrimSpace(expandHome(path))
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func homeDirOrDot() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return home
	}
	return "."
}

// NormalizeImageRef ensures an image reference is fully qualified for containerd.
func NormalizeImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	firstSlash := strings.Index(ref, "/")
	if firstSlash == -1 {
		return "docker.io/library/" + ref
	}
	firstSegment := ref[:firstSlash]
	if strings.Contains(firstSegment, ".") || strings.Contains(firstSegment, ":") || firstSegment == "localhost" {
		return ref
	}
	return "docker.io/" + ref
}

func WorkspaceImagePullCandidates(ref string) []string {
	normalized := NormalizeImageRef(ref)
	if normalized == "" {
		return nil
	}
	if normalized == NormalizeImageRef(DefaultWorkspaceImage) {
		return []string{normalized, DefaultWorkspaceMirrorImage}
	}
	return []string{normalized}
}

type PostgresConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password" json:"-"`
	Database string `toml:"database"`
	SSLMode  string `toml:"sslmode"`
}

type QdrantConfig struct {
	BaseURL        string `toml:"base_url"`
	APIKey         string `toml:"api_key" json:"-"`
	TimeoutSeconds int    `toml:"timeout_seconds"`
}

type SparseConfig struct {
	BaseURL string `toml:"base_url"`
}

const DefaultProvidersDir = "conf/providers"

type RegistryConfig struct {
	ProvidersDir string `toml:"providers_dir"`
}

// ProvidersPath returns the configured providers directory or the default.
func (c RegistryConfig) ProvidersPath() string {
	if c.ProvidersDir != "" {
		return absPath(c.ProvidersDir)
	}
	return absPath(DefaultProvidersDir)
}

const (
	DefaultSupermarketBaseURL     = "https://supermarket.memoh.ai"
	DefaultOAuthClientsConfigPath = "conf/oauth-clients.toml"
)

type SupermarketConfig struct {
	BaseURL string `toml:"base_url"`
}

func (c SupermarketConfig) GetBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultSupermarketBaseURL
}

type OAuthClientsConfig struct {
	ConfigPath string `toml:"config_path"`
}

func (c OAuthClientsConfig) Path() string {
	if strings.TrimSpace(c.ConfigPath) != "" {
		return absPath(c.ConfigPath)
	}
	return absPath(DefaultOAuthClientsConfigPath)
}

func Load(path string) (Config, error) {
	defaultWorkspace := WorkspaceConfig{
		DefaultImage: DefaultBaseImage,
		DataRoot:     DefaultDataRoot,
		CNIBinaryDir: DefaultCNIBinaryDir,
		CNIConfigDir: DefaultCNIConfigDir,
	}
	cfg := Config{
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Server: ServerConfig{
			Addr: DefaultHTTPAddr,
		},
		Admin: AdminConfig{
			Username: "admin",
			Password: "change-your-password-here",
			Email:    "you@example.com",
		},
		Auth: AuthConfig{
			JWTExpiresIn: DefaultJWTExpiresIn,
		},
		Agent: AgentConfig{
			ToolOutputMaxBytes:  DefaultAgentToolOutputBytes,
			ToolOutputMaxLines:  DefaultAgentToolOutputLines,
			SystemFilesMaxBytes: DefaultAgentSystemFilesBytes,
		},
		Timezone: DefaultTimezone,
		Database: DatabaseConfig{
			Driver: DefaultDatabaseDriver,
		},
		Container: ContainerConfig{
			Backend:         "",
			WorkspaceConfig: defaultWorkspace,
		},
		Containerd: ContainerdConfig{
			SocketPath:  DefaultSocketPath,
			Namespace:   DefaultNamespace,
			RuntimeType: DefaultContainerdRuntimeType,
		},
		Workspace: defaultWorkspace,
		Postgres: PostgresConfig{
			Host:     DefaultPGHost,
			Port:     DefaultPGPort,
			User:     DefaultPGUser,
			Database: DefaultPGDatabase,
			SSLMode:  DefaultPGSSLMode,
		},
	}

	if path == "" {
		path = DefaultConfigPath
	}
	path = filepath.Clean(path)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cfg.applyBridgeTLSEnvOverrides()
			if err := cfg.validate(); err != nil {
				return cfg, err
			}
			cfg.resolvePaths()
			return cfg, nil
		}
		return cfg, err
	}

	//nolint:gosec // config path is intentionally user-configurable
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	var raw struct {
		Container map[string]any `toml:"container"`
		Workspace map[string]any `toml:"workspace"`
		MCP       map[string]any `toml:"mcp"`
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return cfg, err
	}
	if raw.MCP != nil {
		if raw.Workspace != nil {
			return cfg, errors.New("config uses both [mcp] and [workspace]; remove [mcp] and move workspace fields into [container]")
		}
		return cfg, errors.New("config section [mcp] has been replaced by workspace fields in [container]; update your config.toml and restart")
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, err
	}
	if raw.Workspace != nil && containerHasWorkspaceFields(raw.Container) {
		return cfg, errors.New("config uses workspace fields in both [container] and [workspace]; move workspace fields into [container] and remove [workspace]")
	}
	if raw.Workspace != nil {
		cfg.Container.WorkspaceConfig = cfg.Workspace
	} else {
		cfg.Workspace = cfg.Container.WorkspaceConfig
	}
	cfg.applyBridgeTLSEnvOverrides()
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	cfg.resolvePaths()

	return cfg, nil
}

func (cfg Config) validate() error {
	if cfg.Database.DriverOrDefault() != DefaultDatabaseDriver {
		return fmt.Errorf("unsupported database driver %q", cfg.Database.DriverOrDefault())
	}
	if err := cfg.WebhookTunnel.Validate(); err != nil {
		return err
	}
	return nil
}

func (cfg *Config) applyBridgeTLSEnvOverrides() {
	if value := strings.TrimSpace(os.Getenv("MEMOH_INSTANCE_ID")); value != "" {
		cfg.InstanceID = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_BRIDGE_TLS_MODE")); value != "" {
		cfg.BridgeTLS.Mode = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_BRIDGE_TLS_SERVER_DIR")); value != "" {
		cfg.BridgeTLS.ServerDir = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_BRIDGE_TLS_BRIDGE_DIR")); value != "" {
		cfg.BridgeTLS.BridgeDir = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_BRIDGE_TLS_SERVER_NAME")); value != "" {
		cfg.BridgeTLS.ServerName = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_WEBHOOK_TUNNEL_MODE")); value != "" {
		cfg.WebhookTunnel.Mode = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_WEBHOOK_PUBLIC_BASE_URL")); value != "" {
		cfg.WebhookTunnel.PublicBaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_WEBHOOK_TUNNEL_LISTEN_ADDR")); value != "" {
		cfg.WebhookTunnel.ListenAddr = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_CLOUDFLARED_BIN")); value != "" {
		cfg.WebhookTunnel.CloudflaredPath = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_WEBHOOK_TUNNEL_TARGET_URL")); value != "" {
		cfg.WebhookTunnel.TargetURL = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_WEBHOOK_TUNNEL_METRICS_ADDR")); value != "" {
		cfg.WebhookTunnel.MetricsAddr = value
	}
	if value := strings.TrimSpace(os.Getenv("MEMOH_WEBHOOK_TUNNEL_METRICS_URL")); value != "" {
		cfg.WebhookTunnel.MetricsURL = value
	}
}

func (cfg *Config) resolvePaths() {
	cfg.Container.DataRoot = cfg.Container.DataRootPath()
	cfg.Container.RuntimeDir = cfg.Container.RuntimePath()
	cfg.Workspace = cfg.Container.WorkspaceConfig
	if strings.TrimSpace(cfg.Local.MetadataRoot) != "" {
		cfg.Local.MetadataRoot = cfg.Local.MetadataPath(cfg.Workspace.DataRoot)
	}
	if strings.TrimSpace(cfg.Registry.ProvidersDir) != "" {
		cfg.Registry.ProvidersDir = cfg.Registry.ProvidersPath()
	}
	if strings.TrimSpace(cfg.OAuthClients.ConfigPath) != "" {
		cfg.OAuthClients.ConfigPath = cfg.OAuthClients.Path()
	}
}

func containerHasWorkspaceFields(values map[string]any) bool {
	for _, key := range []string{
		"registry",
		"default_image",
		"image_pull_policy",
		"snapshotter",
		"data_root",
		"cni_bin_dir",
		"cni_conf_dir",
		"runtime_dir",
	} {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}
