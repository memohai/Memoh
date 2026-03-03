package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	DefaultConfigPath       = "config.toml"
	DefaultHTTPAddr         = ":8080"
	DefaultNamespace        = "default"
	DefaultSocketPath       = "/run/containerd/containerd.sock"
	DefaultMCPImage         = "memohai/mcp:latest"
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
	MCPGRPCPort             = 9090
)

type Config struct {
	Log          LogConfig          `toml:"log"`
	Server       ServerConfig       `toml:"server"`
	Admin        AdminConfig        `toml:"admin"`
	Auth         AuthConfig         `toml:"auth"`
	Containerd   ContainerdConfig   `toml:"containerd"`
	MCP          MCPConfig          `toml:"mcp"`
	Browser      BrowserConfig      `toml:"browser"`
	Postgres     PostgresConfig     `toml:"postgres"`
	Qdrant       QdrantConfig       `toml:"qdrant"`
	AgentGateway AgentGatewayConfig `toml:"agent_gateway"`
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

type MCPConfig struct {
	Registry     string `toml:"registry"`
	Image        string `toml:"image"`
	Snapshotter  string `toml:"snapshotter"`
	DataRoot     string `toml:"data_root"`
	CNIBinaryDir string `toml:"cni_bin_dir"`
	CNIConfigDir string `toml:"cni_conf_dir"`
}

type BrowserConfig struct {
	ServerBaseURL        string `toml:"server_base_url"`
	ServerAPIKey         string `toml:"server_api_key"`
	MaxSessionsPerBot    int32  `toml:"max_sessions_per_bot"`
	IdleTTLSeconds       int32  `toml:"idle_ttl_seconds"`
	MaxIdleTTLSeconds    int32  `toml:"max_idle_ttl_seconds"`
	ActionTimeoutSeconds int32  `toml:"action_timeout_seconds"`
	MaxActionsPerSession int32  `toml:"max_actions_per_session"`
}

func (c *BrowserConfig) ApplyDefaults() {
	if strings.TrimSpace(c.ServerBaseURL) == "" {
		c.ServerBaseURL = "http://127.0.0.1:8090"
	}
	if c.MaxSessionsPerBot <= 0 {
		c.MaxSessionsPerBot = 1
	}
	if c.IdleTTLSeconds <= 0 {
		c.IdleTTLSeconds = 600
	}
	if c.MaxIdleTTLSeconds <= 0 {
		c.MaxIdleTTLSeconds = 3600
	}
	if c.ActionTimeoutSeconds <= 0 {
		c.ActionTimeoutSeconds = 20
	}
	if c.MaxActionsPerSession <= 0 {
		c.MaxActionsPerSession = 50
	}
}

// ImageRef returns the fully qualified image reference, prepending the
// registry mirror when configured and normalizing for containerd compatibility.
// Containerd requires a fully-qualified domain in image references — short
// Docker Hub names like "memohai/mcp:latest" are misinterpreted as hosts.
func (c MCPConfig) ImageRef() string {
	img := c.Image
	if img == "" {
		img = DefaultMCPImage
	}
	if c.Registry != "" {
		return c.Registry + "/" + img
	}
	return NormalizeImageRef(img)
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
		MCP: MCPConfig{
			Image:        DefaultMCPImage,
			DataRoot:     DefaultDataRoot,
			CNIBinaryDir: DefaultCNIBinaryDir,
			CNIConfigDir: DefaultCNIConfigDir,
		},
		Browser: BrowserConfig{
			ServerBaseURL:        "http://127.0.0.1:8090",
			MaxSessionsPerBot:    1,
			IdleTTLSeconds:       600,
			MaxIdleTTLSeconds:    3600,
			ActionTimeoutSeconds: 20,
			MaxActionsPerSession: 50,
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
	}

	if path == "" {
		path = DefaultConfigPath
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	cfg.Browser.ApplyDefaults()

	return cfg, nil
}
