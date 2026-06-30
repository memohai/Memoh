package boot

import (
	"testing"

	"github.com/memohai/memoh/internal/config"
)

func validRuntimeConfig() config.Config {
	return config.Config{
		Auth: config.AuthConfig{
			JWTSecret:    "secret",
			JWTExpiresIn: "24h",
		},
		Timezone: config.DefaultTimezone,
		Container: config.ContainerConfig{
			Backend: "containerd",
		},
		Containerd: config.ContainerdConfig{
			SocketPath: "/run/containerd/containerd.sock",
			Namespace:  "default",
		},
		Server: config.ServerConfig{
			Addr: ":8080",
		},
	}
}

func TestProvideRuntimeConfigResolvesTimezoneSource(t *testing.T) {
	tests := []struct {
		name     string
		configTZ string
		envTZ    string
		want     string
	}{
		{
			name:     "uses configured timezone",
			configTZ: "Asia/Shanghai",
			want:     "Asia/Shanghai",
		},
		{
			name:     "TZ env overrides configured timezone",
			configTZ: "UTC",
			envTZ:    "Asia/Tokyo",
			want:     "Asia/Tokyo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envTZ != "" {
				t.Setenv("TZ", tc.envTZ)
			}
			cfg := validRuntimeConfig()
			cfg.Timezone = tc.configTZ

			rc, err := ProvideRuntimeConfig(cfg)
			if err != nil {
				t.Fatalf("ProvideRuntimeConfig returned error: %v", err)
			}
			if rc.Timezone != tc.want {
				t.Fatalf("Timezone = %q, want %q", rc.Timezone, tc.want)
			}
			if rc.TimezoneLocation == nil {
				t.Fatal("TimezoneLocation is nil")
			}
		})
	}
}

func TestProvideRuntimeConfigRequiresContainerBackend(t *testing.T) {
	cfg := validRuntimeConfig()
	cfg.Container.Backend = ""
	if _, err := ProvideRuntimeConfig(cfg); err == nil {
		t.Fatal("expected missing container backend error")
	}
}

func TestProvideRuntimeConfigBackendIgnoresEnvOverride(t *testing.T) {
	t.Setenv("CONTAINER_BACKEND", "apple")
	cfg := validRuntimeConfig()
	cfg.Container.Backend = "docker"
	rc, err := ProvideRuntimeConfig(cfg)
	if err != nil {
		t.Fatalf("ProvideRuntimeConfig returned error: %v", err)
	}
	if rc.ContainerBackend != "docker" {
		t.Fatalf("ContainerBackend = %q, want docker", rc.ContainerBackend)
	}
}
