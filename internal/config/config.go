package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	DefaultConfigPath       = "config.toml"
	DefaultHTTPAddr         = ":8080"
	DefaultNamespace        = "default"
	DefaultSocketPath       = "/run/containerd/containerd.sock"
	DefaultDataRoot         = "data"
	DefaultDataMount        = "/data"
	DefaultCNIBinaryDir     = "/opt/cni/bin"
	DefaultCNIConfigDir     = "/etc/cni/net.d"
	DefaultJWTExpiresIn     = "24h"
	DefaultDatabaseDriver   = "postgres"
	DefaultPGHost           = "127.0.0.1"
	DefaultPGPort           = 5432
	DefaultPGUser           = "postgres"
	DefaultPGDatabase       = "memoh"
	DefaultPGSSLMode        = "disable"
	DefaultSQLitePath       = "data/memoh.db"
	DefaultSQLiteBusyMS     = 5000
	DefaultQdrantURL        = "http://127.0.0.1:6334"
	DefaultQdrantCollection = "memory"
	DefaultRuntimeDir       = "/opt/memoh/runtime"
	DefaultBaseImage        = "debian:bookworm-slim"
	DefaultVNCImage         = "memohai/vnc:debian"
	DefaultVNCMirrorImage   = "memoh.cn/memohai/vnc:debian"
	DefaultTimezone         = "UTC"

	ImagePullPolicyIfNotPresent = "if_not_present"
	ImagePullPolicyAlways       = "always"
	ImagePullPolicyNever        = "never"
)

type Config struct {
	Log         LogConfig         `toml:"log"`
	Server      ServerConfig      `toml:"server"`
	Admin       AdminConfig       `toml:"admin"`
	Auth        AuthConfig        `toml:"auth"`
	Timezone    string            `toml:"timezone"`
	Database    DatabaseConfig    `toml:"database"`
	Container   ContainerConfig   `toml:"container"`
	Containerd  ContainerdConfig  `toml:"containerd"`
	Docker      DockerConfig      `toml:"docker"`
	Kubernetes  KubernetesConfig  `toml:"kubernetes"`
	Apple       AppleConfig       `toml:"apple"`
	Local       LocalConfig       `toml:"local"`
	Workspace   WorkspaceConfig   `toml:"workspace"`
	Postgres    PostgresConfig    `toml:"postgres"`
	SQLite      SQLiteConfig      `toml:"sqlite"`
	Qdrant      QdrantConfig      `toml:"qdrant"`
	Sparse      SparseConfig      `toml:"sparse"`
	Registry    RegistryConfig    `toml:"registry"`
	Supermarket SupermarketConfig `toml:"supermarket"`
}

type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

type ServerConfig struct {
	Addr string `toml:"addr"`
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
	SocketPath string `toml:"socket_path"`
	Namespace  string `toml:"namespace"`
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

type KubernetesConfig struct {
	Namespace          string `toml:"namespace"`
	Kubeconfig         string `toml:"kubeconfig"`
	InCluster          bool   `toml:"in_cluster"`
	ServiceAccountName string `toml:"service_account_name"`
	ImagePullSecret    string `toml:"image_pull_secret"`
	PVCStorageClass    string `toml:"pvc_storage_class"`
	PVCSize            string `toml:"pvc_size"`
	BridgePort         int    `toml:"bridge_port"`
}

func (c KubernetesConfig) EffectiveNamespace() string {
	if strings.TrimSpace(c.Namespace) != "" {
		return strings.TrimSpace(c.Namespace)
	}
	return DefaultNamespace
}

func (c KubernetesConfig) EffectivePVCSize() string {
	if strings.TrimSpace(c.PVCSize) != "" {
		return strings.TrimSpace(c.PVCSize)
	}
	return "10Gi"
}

func (c KubernetesConfig) EffectiveBridgePort() int {
	if c.BridgePort > 0 {
		return c.BridgePort
	}
	return 9090
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
	if normalized == NormalizeImageRef(DefaultVNCImage) {
		return []string{normalized, DefaultVNCMirrorImage}
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

type SQLiteConfig struct {
	Path          string `toml:"path"`
	DSN           string `toml:"dsn"`
	WAL           bool   `toml:"wal"`
	BusyTimeoutMS int    `toml:"busy_timeout_ms"`
}

func (c SQLiteConfig) PathOrDefault() string {
	if path := strings.TrimSpace(c.Path); path != "" {
		return absPath(path)
	}
	return absPath(DefaultSQLitePath)
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

const DefaultSupermarketBaseURL = "https://supermarket.memoh.ai"

type SupermarketConfig struct {
	BaseURL string `toml:"base_url"`
}

func (c SupermarketConfig) GetBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultSupermarketBaseURL
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
		Timezone: DefaultTimezone,
		Database: DatabaseConfig{
			Driver: DefaultDatabaseDriver,
		},
		Container: ContainerConfig{
			Backend:         "",
			WorkspaceConfig: defaultWorkspace,
		},
		Containerd: ContainerdConfig{
			SocketPath: DefaultSocketPath,
			Namespace:  DefaultNamespace,
		},
		Kubernetes: KubernetesConfig{
			Namespace:  DefaultNamespace,
			InCluster:  true,
			PVCSize:    "10Gi",
			BridgePort: 9090,
		},
		Workspace: defaultWorkspace,
		Postgres: PostgresConfig{
			Host:     DefaultPGHost,
			Port:     DefaultPGPort,
			User:     DefaultPGUser,
			Database: DefaultPGDatabase,
			SSLMode:  DefaultPGSSLMode,
		},
		SQLite: SQLiteConfig{
			Path:          DefaultSQLitePath,
			WAL:           true,
			BusyTimeoutMS: DefaultSQLiteBusyMS,
		},
	}

	if path == "" {
		path = DefaultConfigPath
	}
	path = filepath.Clean(path)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
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
	cfg.resolvePaths()

	return cfg, nil
}

func (cfg *Config) resolvePaths() {
	cfg.Container.DataRoot = cfg.Container.DataRootPath()
	cfg.Container.RuntimeDir = cfg.Container.RuntimePath()
	cfg.Workspace = cfg.Container.WorkspaceConfig
	if strings.TrimSpace(cfg.Local.MetadataRoot) != "" {
		cfg.Local.MetadataRoot = cfg.Local.MetadataPath(cfg.Workspace.DataRoot)
	}
	if strings.TrimSpace(cfg.SQLite.DSN) == "" {
		cfg.SQLite.Path = cfg.SQLite.PathOrDefault()
	}
	if strings.TrimSpace(cfg.Registry.ProvidersDir) != "" {
		cfg.Registry.ProvidersDir = cfg.Registry.ProvidersPath()
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
