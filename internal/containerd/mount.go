package containerd

import (
	"context"
	"fmt"
	"os"

	"github.com/containerd/containerd/v2/core/mount"
)

type MountedSnapshot struct {
	Dir     string
	Info    ContainerInfo
	Unmount func() error
}

// MountContainerSnapshot mounts the active snapshot for a container.
func MountContainerSnapshot(ctx context.Context, service Service, containerID string) (*MountedSnapshot, error) {
	if containerID == "" {
		return nil, ErrInvalidArgument
	}

	info, err := service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}

	mountInfos, err := service.SnapshotMounts(ctx, info.Snapshotter, info.SnapshotKey)
	if err != nil {
		return nil, err
	}

	mounts := make([]mount.Mount, len(mountInfos))
	for i, m := range mountInfos {
		mounts[i] = mount.Mount{
			Type:    m.Type,
			Source:  m.Source,
			Options: m.Options,
		}
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

	mountInfos, err := service.SnapshotMounts(ctx, snapshotter, key)
	if err != nil {
		return "", nil, err
	}

	mounts := make([]mount.Mount, len(mountInfos))
	for i, m := range mountInfos {
		mounts[i] = mount.Mount{
			Type:    m.Type,
			Source:  m.Source,
			Options: m.Options,
		}
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
