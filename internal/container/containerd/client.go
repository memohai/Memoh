package containerd

import (
	"context"

	containerdclient "github.com/containerd/containerd/v2/client"
)

func NewClient(_ context.Context, socketPath string) (*containerdclient.Client, error) {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	return containerdclient.New(socketPath)
}
