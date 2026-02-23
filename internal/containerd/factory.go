package containerd

import (
	"context"
	"fmt"
	"log/slog"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/memohai/memoh/internal/config"
)

const (
	DefaultSocketPath = "/run/containerd/containerd.sock"
	DefaultNamespace  = "default"

	BackendContainerd = "containerd"
	BackendApple      = "apple"
)

type ClientFactory interface {
	New(ctx context.Context) (*containerd.Client, error)
}

type DefaultClientFactory struct {
	SocketPath string
}

func (f DefaultClientFactory) New(_ context.Context) (*containerd.Client, error) {
	socket := f.SocketPath
	if socket == "" {
		socket = DefaultSocketPath
	}
	return containerd.New(socket)
}

// ProvideService creates the appropriate Service based on the backend type.
func ProvideService(ctx context.Context, log *slog.Logger, cfg config.Config, backend string) (Service, func(), error) {
	switch backend {
	case BackendApple:
		svc, err := NewAppleService(ctx, log, AppleServiceConfig{
			SocketPath: cfg.Containerd.Socktainer.SocketPath,
			BinaryPath: cfg.Containerd.Socktainer.BinaryPath,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create apple container service: %w", err)
		}
		cleanup := func() { _ = svc.Close() }
		return svc, cleanup, nil

	default:
		factory := DefaultClientFactory{SocketPath: cfg.Containerd.SocketPath}
		client, err := factory.New(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("connect containerd: %w", err)
		}
		svc := NewDefaultService(log, client, cfg)
		cleanup := func() { _ = client.Close() }
		return svc, cleanup, nil
	}
}
