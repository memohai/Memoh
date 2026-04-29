package provider

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/config"
	containerapi "github.com/memohai/memoh/internal/container"
)

func TestProvideServiceDockerSlot(t *testing.T) {
	svc, cleanup, err := ProvideService(context.Background(), slog.Default(), config.Config{}, containerapi.BackendDocker)
	if err != nil {
		t.Fatalf("ProvideService docker returned error: %v", err)
	}
	defer cleanup()
	imageSvc, ok := svc.(containerapi.ImageService)
	if !ok {
		t.Fatal("docker service should expose optional ImageService")
	}
	if _, err := imageSvc.GetImage(context.Background(), "memohai/definitely-missing:test"); !containerapi.IsNotFound(err) {
		t.Fatalf("docker GetImage error = %v, want not found", err)
	}
}

func TestProvideServiceKubernetesSlot(t *testing.T) {
	svc, cleanup, err := ProvideService(context.Background(), slog.Default(), config.Config{}, containerapi.BackendKubernetes)
	if err != nil {
		t.Fatalf("ProvideService kubernetes returned error: %v", err)
	}
	defer cleanup()
	if imageSvc, ok := svc.(containerapi.ImageService); ok {
		if _, err := imageSvc.GetImage(context.Background(), "debian"); !errors.Is(err, containerapi.ErrNotSupported) {
			t.Fatalf("kubernetes GetImage error = %v, want ErrNotSupported", err)
		}
	}
}

func TestProvideServiceRejectsUnknownBackend(t *testing.T) {
	if _, _, err := ProvideService(context.Background(), slog.Default(), config.Config{}, "unknown"); err == nil {
		t.Fatal("expected unknown backend error")
	}
}
