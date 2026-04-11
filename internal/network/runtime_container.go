package network

import (
	"context"
	"strings"

	ctr "github.com/memohai/memoh/internal/containerd"
)

type containerRuntimeService interface {
	SetupNetwork(ctx context.Context, req ctr.NetworkRequest) (ctr.NetworkResult, error)
	RemoveNetwork(ctx context.Context, req ctr.NetworkRequest) error
}

type containerRuntime struct {
	svc  containerRuntimeService
	desc RuntimeDescriptor
}

// NewContainerRuntimeFromBackend wraps the current runtime network service with
// the network package's runtime abstraction.
func NewContainerRuntimeFromBackend(backend string, svc containerRuntimeService) Runtime {
	return &containerRuntime{
		svc:  svc,
		desc: descriptorForBackend(backend),
	}
}

func (r *containerRuntime) Kind() string {
	return r.desc.Kind
}

func (r *containerRuntime) Descriptor() RuntimeDescriptor {
	return r.desc
}

func (r *containerRuntime) EnsureNetwork(ctx context.Context, req RuntimeNetworkRequest) (RuntimeNetworkStatus, error) {
	result, err := r.svc.SetupNetwork(ctx, ctr.NetworkRequest{
		ContainerID: req.ContainerID,
		NetNSPath:   req.NetNSPath,
		PID:         req.PID,
		CNIBinDir:   req.CNIBinDir,
		CNIConfDir:  req.CNIConfDir,
	})
	if err != nil {
		return RuntimeNetworkStatus{}, err
	}
	ip := strings.TrimSpace(result.IP)
	return RuntimeNetworkStatus{
		Attached: ip != "",
		IP:       ip,
	}, nil
}

func (r *containerRuntime) RemoveNetwork(ctx context.Context, req RuntimeNetworkRequest) error {
	return r.svc.RemoveNetwork(ctx, ctr.NetworkRequest{
		ContainerID: req.ContainerID,
		NetNSPath:   req.NetNSPath,
		PID:         req.PID,
		CNIBinDir:   req.CNIBinDir,
		CNIConfDir:  req.CNIConfDir,
	})
}

func (*containerRuntime) StatusNetwork(context.Context, RuntimeNetworkRequest) (RuntimeNetworkStatus, error) {
	return RuntimeNetworkStatus{}, ErrNotSupported
}

func descriptorForBackend(backend string) RuntimeDescriptor {
	switch normalizeKind(backend) {
	case "apple":
		return RuntimeDescriptor{
			Kind:         "apple",
			DisplayName:  "Apple Container",
			Capabilities: RuntimeCapabilities{},
		}
	default:
		return RuntimeDescriptor{
			Kind:        "containerd",
			DisplayName: "containerd",
			Capabilities: RuntimeCapabilities{
				JoinNamespacePath: true,
				CNI:               true,
				Devices:           true,
				Capabilities:      true,
				Privileged:        true,
			},
		}
	}
}
