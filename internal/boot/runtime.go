// Package boot provides runtime configuration and dependency wiring for the agent.
package boot

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
)

// RuntimeConfig holds parsed runtime settings (JWT, server address, containerd socket).
// Values may be overridden by environment variables (e.g. HTTP_ADDR, CONTAINERD_SOCKET).
type RuntimeConfig struct {
	JwtSecret            string
	JwtExpiresIn         time.Duration
	ServerAddr           string
	ContainerdSocketPath string
}

// ProvideRuntimeConfig builds RuntimeConfig from the given config and applies env overrides.
func ProvideRuntimeConfig(cfg config.Config) (*RuntimeConfig, error) {
	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		return nil, errors.New("jwt secret is required")
	}

	jwtExpiresIn, err := time.ParseDuration(cfg.Auth.JWTExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("invalid jwt expires in: %w", err)
	}

	ret := &RuntimeConfig{
		JwtSecret:            cfg.Auth.JWTSecret,
		JwtExpiresIn:         jwtExpiresIn,
		ServerAddr:           cfg.Server.Addr,
		ContainerdSocketPath: cfg.Containerd.SocketPath,
	}

	if value := os.Getenv("HTTP_ADDR"); value != "" {
		ret.ServerAddr = value
	}

	if value := os.Getenv("CONTAINERD_SOCKET"); value != "" {
		ret.ContainerdSocketPath = value
	}
	return ret, nil
}
