package provider

import (
	"context"
	"fmt"
	"log/slog"

	containerdclient "github.com/containerd/containerd/v2/client"

	"github.com/memohai/memoh/internal/config"
	containerapi "github.com/memohai/memoh/internal/container"
	appleadapter "github.com/memohai/memoh/internal/container/apple"
	containerdadapter "github.com/memohai/memoh/internal/container/containerd"
	dockeradapter "github.com/memohai/memoh/internal/container/docker"
	k8sadapter "github.com/memohai/memoh/internal/container/k8s"
)

type ClientFactory interface {
	New(ctx context.Context) (*containerdclient.Client, error)
}

type DefaultClientFactory struct {
	SocketPath string
}

func (f DefaultClientFactory) New(_ context.Context) (*containerdclient.Client, error) {
	socket := f.SocketPath
	if socket == "" {
		socket = containerapi.DefaultSocketPath
	}
	return containerdclient.New(socket)
}

// ProvideService creates the appropriate Service based on the backend type.
func ProvideService(ctx context.Context, log *slog.Logger, cfg config.Config, backend string) (containerapi.Service, func(), error) {
	switch backend {
	case containerapi.BackendApple:
		svc, err := appleadapter.NewService(ctx, log, appleadapter.ServiceConfig{
			SocketPath: cfg.Containerd.Socktainer.SocketPath,
			BinaryPath: cfg.Containerd.Socktainer.BinaryPath,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create apple container service: %w", err)
		}
		cleanup := func() { _ = svc.Close() }
		return svc, cleanup, nil
	case containerapi.BackendDocker:
		svc, err := dockeradapter.NewService(log, cfg)
		if err != nil {
			return nil, nil, err
		}
		return svc, func() { _ = svc.Close() }, nil
	case containerapi.BackendKubernetes, containerapi.BackendK8s:
		return k8sadapter.NewService(cfg), func() {}, nil
	case containerapi.BackendContainerd:
		factory := DefaultClientFactory{SocketPath: cfg.Containerd.SocketPath}
		client, err := factory.New(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("connect containerd: %w", err)
		}
		svc := containerdadapter.NewService(log, client, cfg)
		cleanup := func() { _ = client.Close() }
		return svc, cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unsupported container backend %q", backend)
	}
}
