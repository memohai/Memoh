package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
	DefaultPGHost           = "127.0.0.1"
	DefaultPGPort           = 5432
	DefaultPGUser           = "postgres"
	DefaultPGDatabase       = "memoh"
	DefaultPGSSLMode        = "disable"
	DefaultQdrantURL        = "http://127.0.0.1:6334"
	DefaultQdrantCollection = "memory"
	DefaultRuntimeDir       = "/opt/memoh/runtime"
	DefaultBaseImage        = "debian:bookworm-slim"
)

type Config struct {
	Log            LogConfig            `toml:"log"`
	Server         ServerConfig         `toml:"server"`
	Admin          AdminConfig          `toml:"admin"`
	Auth           AuthConfig           `toml:"auth"`
	Containerd     ContainerdConfig     `toml:"containerd"`
	Workspace      WorkspaceConfig      `toml:"workspace"`
	Postgres       PostgresConfig       `toml:"postgres"`
	Qdrant         QdrantConfig         `toml:"qdrant"`
	Sparse         SparseConfig         `toml:"sparse"`
	AgentGateway   AgentGatewayConfig   `toml:"agent_gateway"`
	BrowserGateway BrowserGatewayConfig `toml:"browser_gateway"`
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

type ContainerdConfig struct {
	SocketPath string           `toml:"socket_path"`
	Namespace  string           `toml:"namespace"`
	Socktainer SocktainerConfig `toml:"socktainer"`
}

type SocktainerConfig struct {
	SocketPath string `toml:"socket_path"`
	BinaryPath string `toml:"binary_path"`
}

type WorkspaceConfig struct {
	Registry     string `toml:"registry"`
	DefaultImage string `toml:"default_image"`
	Snapshotter  string `toml:"snapshotter"`
	DataRoot     string `toml:"data_root"`
	CNIBinaryDir string `toml:"cni_bin_dir"`
	CNIConfigDir string `toml:"cni_conf_dir"`
	RuntimeDir   string `toml:"runtime_dir"`
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
		return c.RuntimeDir
	}
	return DefaultRuntimeDir
}

// NormalizeImageRef ensures an image reference is fully qualified for containerd.
func NormalizeImageRef(ref string) string {
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

type AgentGatewayConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

func (c AgentGatewayConfig) BaseURL() string {
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port == 0 {
		port = 8081
	}
	return "http://" + host + ":" + strconv.Itoa(port)
}

type BrowserGatewayConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

func (c BrowserGatewayConfig) BaseURL() string {
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port == 0 {
		port = 8083
	}
	return "http://" + host + ":" + strconv.Itoa(port)
}

func Load(path string) (Config, error) {
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
		Containerd: ContainerdConfig{
			SocketPath: DefaultSocketPath,
			Namespace:  DefaultNamespace,
		},
		Workspace: WorkspaceConfig{
			DefaultImage: DefaultBaseImage,
			DataRoot:     DefaultDataRoot,
			CNIBinaryDir: DefaultCNIBinaryDir,
			CNIConfigDir: DefaultCNIConfigDir,
		},
		Postgres: PostgresConfig{
			Host:     DefaultPGHost,
			Port:     DefaultPGPort,
			User:     DefaultPGUser,
			Database: DefaultPGDatabase,
			SSLMode:  DefaultPGSSLMode,
		},
		AgentGateway: AgentGatewayConfig{
			Host: "127.0.0.1",
			Port: 8081,
		},
		BrowserGateway: BrowserGatewayConfig{
			Host: "127.0.0.1",
			Port: 8083,
		},
	}

	if path == "" {
		path = DefaultConfigPath
	}
	path = filepath.Clean(path)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
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
		Workspace map[string]any `toml:"workspace"`
		MCP       map[string]any `toml:"mcp"`
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return cfg, err
	}
	if raw.MCP != nil {
		if raw.Workspace != nil {
			return cfg, errors.New("config uses both [mcp] and [workspace]; remove [mcp] and keep only [workspace]")
		}
		return cfg, errors.New("config section [mcp] has been renamed to [workspace]; update your config.toml and restart")
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
