package containerd

import (
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/config"
)

func TestNewDefaultServiceDefaultsRuntimeType(t *testing.T) {
	t.Parallel()

	svc := NewDefaultService(slog.New(slog.DiscardHandler), nil, config.Config{})
	if got := svc.runtimeTypeOrDefault(); got != config.DefaultContainerdRuntimeType {
		t.Fatalf("runtime type = %q, want %q", got, config.DefaultContainerdRuntimeType)
	}
}

func TestNewDefaultServiceUsesConfiguredRuntimeType(t *testing.T) {
	t.Parallel()

	svc := NewDefaultService(slog.New(slog.DiscardHandler), nil, config.Config{
		Containerd: config.ContainerdConfig{RuntimeType: "io.containerd.kata.v2"},
	})
	if got := svc.runtimeTypeOrDefault(); got != "io.containerd.kata.v2" {
		t.Fatalf("runtime type = %q, want io.containerd.kata.v2", got)
	}
}
