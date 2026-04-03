// Package storage defines the Provider interface for object storage backends.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrContainerFileNotSupported is returned when no underlying provider
// implements ContainerFileOpener.
var ErrContainerFileNotSupported = errors.New("provider does not support container file reading")

// Provider abstracts object storage operations.
type Provider interface {
	// Put writes data to storage under the given key.
	Put(ctx context.Context, key string, reader io.Reader) error
	// Open returns a reader for the given storage key.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the object at key.
	Delete(ctx context.Context, key string) error
	// AccessPath returns a consumer-accessible reference for a storage key.
	// The format depends on the backend (e.g. container path, signed URL).
	AccessPath(key string) string
}

// ContainerFileOpener is an optional interface that providers can implement
// to open arbitrary files from a bot's container data directory.
type ContainerFileOpener interface {
	OpenContainerFile(ctx context.Context, botID, containerPath string) (io.ReadCloser, error)
}

// PrefixLister is an optional interface for providers that can list keys
// sharing a common prefix (e.g. directory listing on a filesystem backend).
type PrefixLister interface {
	ListPrefix(ctx context.Context, prefix string) ([]string, error)
}
