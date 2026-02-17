// Package containerd provides the containerd client factory and service abstraction.
package containerd

import (
	"context"

	containerd "github.com/containerd/containerd/v2/client"
)

// Default socket path and namespace when not set in config.
const (
	DefaultSocketPath = "/run/containerd/containerd.sock"
	DefaultNamespace  = "default"
)

// ClientFactory creates a containerd client (e.g. from socket path).
type ClientFactory interface {
	New(ctx context.Context) (*containerd.Client, error)
}

// DefaultClientFactory creates a client using SocketPath (or DefaultSocketPath if empty).
type DefaultClientFactory struct {
	SocketPath string
}

// New returns a new containerd client connected to the configured socket.
func (f DefaultClientFactory) New(_ context.Context) (*containerd.Client, error) {
	socket := f.SocketPath
	if socket == "" {
		socket = DefaultSocketPath
	}
	return containerd.New(socket)
}
