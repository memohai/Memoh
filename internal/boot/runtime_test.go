package boot

import (
	"testing"

	"github.com/memohai/memoh/internal/config"
)

func TestProvideRuntimeConfig_DefaultTimezone(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Auth: config.AuthConfig{
			JWTSecret:    "secret",
			JWTExpiresIn: "24h",
		},
		Timezone: config.DefaultTimezone,
		Containerd: config.ContainerdConfig{
			SocketPath: "/run/containerd/containerd.sock",
			Namespace:  "default",
		},
		Server: config.ServerConfig{
			Addr: ":8080",
		},
	}

	rc, err := ProvideRuntimeConfig(cfg)
	if err != nil {
		t.Fatalf("ProvideRuntimeConfig returned error: %v", err)
	}
	if rc.Timezone != config.DefaultTimezone {
		t.Fatalf("Timezone = %q, want %q", rc.Timezone, config.DefaultTimezone)
	}
	if rc.TimezoneLocation == nil {
		t.Fatal("TimezoneLocation is nil")
	}
	if rc.TimezoneLocation.String() != config.DefaultTimezone {
		t.Fatalf("TimezoneLocation = %q, want %q", rc.TimezoneLocation.String(), config.DefaultTimezone)
	}
}

func TestProvideRuntimeConfig_ResolvesTZEnv(t *testing.T) {
	t.Setenv("TZ", "Asia/Tokyo")

	cfg := config.Config{
		Auth: config.AuthConfig{
			JWTSecret:    "secret",
			JWTExpiresIn: "24h",
		},
		Timezone: "UTC",
		Containerd: config.ContainerdConfig{
			SocketPath: "/run/containerd/containerd.sock",
			Namespace:  "default",
		},
		Server: config.ServerConfig{
			Addr: ":8080",
		},
	}

	rc, err := ProvideRuntimeConfig(cfg)
	if err != nil {
		t.Fatalf("ProvideRuntimeConfig returned error: %v", err)
	}
	if rc.Timezone != "Asia/Tokyo" {
		t.Fatalf("Timezone = %q, want Asia/Tokyo", rc.Timezone)
	}
	if rc.TimezoneLocation == nil {
		t.Fatal("TimezoneLocation is nil")
	}
	if rc.TimezoneLocation.String() != "Asia/Tokyo" {
		t.Fatalf("TimezoneLocation = %q, want Asia/Tokyo", rc.TimezoneLocation.String())
	}
}
