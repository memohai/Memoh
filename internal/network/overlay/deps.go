package overlay

import (
	netctl "github.com/memohai/memoh/internal/network"
	"github.com/memohai/memoh/internal/network/kubeapi"
	"github.com/memohai/memoh/internal/network/overlay/internal/sidecar"
)

type ProviderDeps struct {
	SidecarRuntime sidecar.Runtime
	KubeRuntime    kubeapi.Runtime
	Runtime        netctl.RuntimeDescriptor
	StateRoot      string
}
