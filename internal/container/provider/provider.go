package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/memohai/memoh/internal/config"
	containerapi "github.com/memohai/memoh/internal/container"
	appleadapter "github.com/memohai/memoh/internal/container/apple"
	containerdadapter "github.com/memohai/memoh/internal/container/containerd"
	dockeradapter "github.com/memohai/memoh/internal/container/docker"
	k8sadapter "github.com/memohai/memoh/internal/container/k8s"
)

// ProvideService creates the appropriate Service based on the backend type.
func ProvideService(ctx context.Context, log *slog.Logger, cfg config.Config, backend string) (containerapi.Service, func(), error) {
	switch backend {
	case containerapi.BackendApple:
		svc, err := appleadapter.NewService(ctx, log, appleadapter.ServiceConfig{
			SocketPath: cfg.Apple.SocketPath,
			BinaryPath: cfg.Apple.BinaryPath,
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
		client, err := containerdadapter.NewClient(ctx, cfg.Containerd.SocketPath)
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
