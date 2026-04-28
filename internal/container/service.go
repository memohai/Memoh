package container

import (
	"context"
	"syscall"
	"time"
)

type PullImageOptions struct {
	Unpack      bool
	Snapshotter string
	OnProgress  func(PullProgress) // optional, nil = no progress reporting
}

type DeleteImageOptions struct {
	Synchronous bool
}

type CreateContainerRequest struct {
	ID          string
	ImageRef    string
	SnapshotID  string
	Snapshotter string
	Labels      map[string]string
	Spec        ContainerSpec
}

type DeleteContainerOptions struct {
	CleanupSnapshot bool
}

type StartTaskOptions struct {
	Terminal bool
}

type StopTaskOptions struct {
	Signal  syscall.Signal
	Timeout time.Duration
	Force   bool
}

type DeleteTaskOptions struct {
	Force bool
}

type SnapshotCommitResult struct {
	VersionSnapshotName string
	ActiveSnapshotName  string
}

type ListTasksOptions struct {
	Filter string
}

// ImageService groups image and registry operations.
type ImageService interface {
	PullImage(ctx context.Context, ref string, opts *PullImageOptions) (ImageInfo, error)
	GetImage(ctx context.Context, ref string) (ImageInfo, error)
	ListImages(ctx context.Context) ([]ImageInfo, error)
	DeleteImage(ctx context.Context, ref string, opts *DeleteImageOptions) error
	// ResolveRemoteDigest fetches only the manifest digest from the registry
	// without downloading any layers. Returns ErrNotSupported on backends that
	// have no concept of a remote registry.
	ResolveRemoteDigest(ctx context.Context, ref string) (string, error)
}

// ContainerService groups container metadata and creation operations.
type ContainerService interface {
	CreateContainer(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error)
	GetContainer(ctx context.Context, id string) (ContainerInfo, error)
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
	DeleteContainer(ctx context.Context, id string, opts *DeleteContainerOptions) error
	ListContainersByLabel(ctx context.Context, key, value string) ([]ContainerInfo, error)
	CreateContainerFromSnapshot(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error)
}

// TaskService groups workload process lifecycle operations.
type TaskService interface {
	StartContainer(ctx context.Context, containerID string, opts *StartTaskOptions) error
	StopContainer(ctx context.Context, containerID string, opts *StopTaskOptions) error
	DeleteTask(ctx context.Context, containerID string, opts *DeleteTaskOptions) error
	GetTaskInfo(ctx context.Context, containerID string) (TaskInfo, error)
	GetContainerMetrics(ctx context.Context, containerID string) (ContainerMetrics, error)
	ListTasks(ctx context.Context, opts *ListTasksOptions) ([]TaskInfo, error)
}

// NetworkService groups runtime network attachment operations.
type NetworkService interface {
	SetupNetwork(ctx context.Context, req NetworkRequest) (NetworkResult, error)
	RemoveNetwork(ctx context.Context, req NetworkRequest) error
	CheckNetwork(ctx context.Context, req NetworkRequest) error
}

// SnapshotService groups snapshot and rootfs operations.
type SnapshotService interface {
	CommitSnapshot(ctx context.Context, snapshotter, name, key string) error
	ListSnapshots(ctx context.Context, snapshotter string) ([]SnapshotInfo, error)
	PrepareSnapshot(ctx context.Context, snapshotter, key, parent string) error
	SnapshotUsage(ctx context.Context, snapshotter, key string) (SnapshotUsage, error)
	SnapshotMounts(ctx context.Context, snapshotter, key string) ([]MountInfo, error)
}

// Service is the workspace-facing container runtime abstraction.
type Service interface {
	ImageService
	ContainerService
	TaskService
	NetworkService
	SnapshotService
}
