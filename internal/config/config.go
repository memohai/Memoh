// Package config loads and exposes application configuration (TOML).
package config

import (
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
)

// Default configuration values used when a field is missing in TOML.
const (
	DefaultConfigPath       = "config.toml"
	DefaultHTTPAddr         = ":8080"
	DefaultNamespace        = "default"
	DefaultSocketPath       = "/run/containerd/containerd.sock"
	DefaultMCPImage         = "docker.io/library/memoh-mcp:latest"
	DefaultDataRoot         = "data"
	DefaultDataMount        = "/data"
	DefaultJWTExpiresIn     = "24h"
	DefaultPGHost           = "127.0.0.1"
	DefaultPGPort           = 5432
	DefaultPGUser           = "postgres"
	DefaultPGDatabase       = "memoh"
	DefaultPGSSLMode        = "disable"
	DefaultQdrantURL        = "http://127.0.0.1:6334"
	DefaultQdrantCollection = "memory"
)

// Config is the root application configuration loaded from TOML.
type Config struct {
	Log          LogConfig          `toml:"log"`
	Server       ServerConfig       `toml:"server"`
	Admin        AdminConfig        `toml:"admin"`
	Auth         AuthConfig         `toml:"auth"`
	Containerd   ContainerdConfig   `toml:"containerd"`
	MCP          MCPConfig          `toml:"mcp"`
	Postgres     PostgresConfig     `toml:"postgres"`
	Qdrant       QdrantConfig       `toml:"qdrant"`
	AgentGateway AgentGatewayConfig `toml:"agent_gateway"`
}

// LogConfig holds logging level and format (e.g. level=info, format=text).
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// ServerConfig holds the HTTP server listen address.
type ServerConfig struct {
	Addr string `toml:"addr"`
}

// AdminConfig holds the initial admin account (username, password, email).
type AdminConfig struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
	Email    string `toml:"email"`
}

// AuthConfig holds JWT secret and token expiry (e.g. 24h).
type AuthConfig struct {
	JWTSecret    string `toml:"jwt_secret"`
	JWTExpiresIn string `toml:"jwt_expires_in"`
}

// ContainerdConfig holds the containerd socket path and namespace.
type ContainerdConfig struct {
	SocketPath string `toml:"socket_path"`
	Namespace  string `toml:"namespace"`
}

// MCPConfig holds MCP container image, snapshotter, and data paths.
type MCPConfig struct {
	Image       string `toml:"image"`
	Snapshotter string `toml:"snapshotter"`
	DataRoot    string `toml:"data_root"`
	DataMount   string `toml:"data_mount"`
}

// PostgresConfig holds PostgreSQL connection parameters.
type PostgresConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	Database string `toml:"database"`
	SSLMode  string `toml:"sslmode"`
}

// QdrantConfig holds Qdrant base URL, API key, collection name, and timeout.
type QdrantConfig struct {
	BaseURL        string `toml:"base_url"`
	APIKey         string `toml:"api_key"`
	Collection     string `toml:"collection"`
	TimeoutSeconds int    `toml:"timeout_seconds"`
}

// AgentGatewayConfig holds the agent gateway host and port.
type AgentGatewayConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

// BaseURL returns the agent gateway base URL (e.g. http://127.0.0.1:8081) from host and port.
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

// Load reads and parses the TOML config file at path and applies default values for missing fields.
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
			Image:     DefaultMCPImage,
			DataRoot:  DefaultDataRoot,
			DataMount: DefaultDataMount,
		},
		Postgres: PostgresConfig{
			Host:     DefaultPGHost,
			Port:     DefaultPGPort,
			User:     DefaultPGUser,
			Database: DefaultPGDatabase,
			SSLMode:  DefaultPGSSLMode,
		},
		Qdrant: QdrantConfig{
			BaseURL:    DefaultQdrantURL,
			Collection: DefaultQdrantCollection,
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

	return cfg, nil
}
