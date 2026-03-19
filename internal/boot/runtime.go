package boot

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/timezone"
)

type RuntimeConfig struct {
	JwtSecret            string `json:"-"`
	JwtExpiresIn         time.Duration
	ServerAddr           string
	ContainerdSocketPath string
	ContainerBackend     string // "containerd" or "apple"
	Timezone             string
	TimezoneLocation     *time.Location
}

func ProvideRuntimeConfig(cfg config.Config) (*RuntimeConfig, error) {
	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		return nil, errors.New("jwt secret is required")
	}

	jwtExpiresIn, err := time.ParseDuration(cfg.Auth.JWTExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("invalid jwt expires in: %w", err)
	}

	backend := "containerd"
	if runtime.GOOS == "darwin" {
		backend = "apple"
	}

	tzName := strings.TrimSpace(cfg.Timezone)
	if envTZ := strings.TrimSpace(os.Getenv("TZ")); envTZ != "" {
		tzName = envTZ
	}
	tzLocation, resolvedTZ, err := timezone.Resolve(tzName)
	if err != nil {
		return nil, err
	}

	ret := &RuntimeConfig{
		JwtSecret:            cfg.Auth.JWTSecret,
		JwtExpiresIn:         jwtExpiresIn,
		ServerAddr:           cfg.Server.Addr,
		ContainerdSocketPath: cfg.Containerd.SocketPath,
		ContainerBackend:     backend,
		Timezone:             resolvedTZ,
		TimezoneLocation:     tzLocation,
	}

	if value := os.Getenv("HTTP_ADDR"); value != "" {
		ret.ServerAddr = value
	}

	if value := os.Getenv("CONTAINERD_SOCKET"); value != "" {
		ret.ContainerdSocketPath = value
	}
	if value := os.Getenv("CONTAINER_BACKEND"); value != "" {
		ret.ContainerBackend = value
	}
	return ret, nil
}
