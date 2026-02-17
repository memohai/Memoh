package containerd

import (
	"context"
	"fmt"
	"os"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/mount"
)

// MountedSnapshot holds the mount directory, container info, and an Unmount function to release it.
type MountedSnapshot struct {
	Dir     string
	Info    containers.Container
	Unmount func() error
}

// MountContainerSnapshot mounts the active snapshot for a container into a temp dir; call Unmount when done.
func MountContainerSnapshot(ctx context.Context, service Service, containerID string) (*MountedSnapshot, error) {
	if containerID == "" {
		return nil, ErrInvalidArgument
	}

	container, err := service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}
	info, err := container.Info(ctx)
	if err != nil {
		return nil, err
	}

	mounts, err := service.SnapshotMounts(ctx, info.Snapshotter, info.SnapshotKey)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "memoh-snapshot-*")
	if err != nil {
		return nil, err
	}

	if err := mount.All(mounts, dir); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	return &MountedSnapshot{
		Dir:  dir,
		Info: info,
		Unmount: func() error {
			if err := mount.UnmountAll(dir, 0); err != nil {
				return fmt.Errorf("unmount snapshot: %w", err)
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("remove snapshot dir: %w", err)
			}
			return nil
		},
	}, nil
}

// MountSnapshot mounts a snapshot by snapshotter/key without a container.
func MountSnapshot(ctx context.Context, service Service, snapshotter, key string) (string, func() error, error) {
	if snapshotter == "" || key == "" {
		return "", nil, ErrInvalidArgument
	}

	mounts, err := service.SnapshotMounts(ctx, snapshotter, key)
	if err != nil {
		return "", nil, err
	}

	dir, err := os.MkdirTemp("", "memoh-snapshot-*")
	if err != nil {
		return "", nil, err
	}

	if err := mount.All(mounts, dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}

	cleanup := func() error {
		if err := mount.UnmountAll(dir, 0); err != nil {
			return fmt.Errorf("unmount snapshot: %w", err)
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove snapshot dir: %w", err)
		}
		return nil
	}

	return dir, cleanup, nil
}
