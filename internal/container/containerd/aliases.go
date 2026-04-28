package containerd

import containerapi "github.com/memohai/memoh/internal/container"

var (
	ErrInvalidArgument = containerapi.ErrInvalidArgument
	ErrNotSupported    = containerapi.ErrNotSupported
)

type (
	PullImageOptions       = containerapi.PullImageOptions
	DeleteImageOptions     = containerapi.DeleteImageOptions
	CreateContainerRequest = containerapi.CreateContainerRequest
	DeleteContainerOptions = containerapi.DeleteContainerOptions
	StartTaskOptions       = containerapi.StartTaskOptions
	StopTaskOptions        = containerapi.StopTaskOptions
	DeleteTaskOptions      = containerapi.DeleteTaskOptions
	SnapshotCommitResult   = containerapi.SnapshotCommitResult
	ListTasksOptions       = containerapi.ListTasksOptions
)

type (
	ImageInfo        = containerapi.ImageInfo
	ContainerInfo    = containerapi.ContainerInfo
	RuntimeInfo      = containerapi.RuntimeInfo
	TaskInfo         = containerapi.TaskInfo
	TaskStatus       = containerapi.TaskStatus
	ContainerMetrics = containerapi.ContainerMetrics
	CPUMetrics       = containerapi.CPUMetrics
	MemoryMetrics    = containerapi.MemoryMetrics
	SnapshotUsage    = containerapi.SnapshotUsage
	SnapshotInfo     = containerapi.SnapshotInfo
	MountInfo        = containerapi.MountInfo
	MountSpec        = containerapi.MountSpec
	ContainerSpec    = containerapi.ContainerSpec
	PullProgress     = containerapi.PullProgress
	LayerStatus      = containerapi.LayerStatus
	NetworkRequest   = containerapi.NetworkRequest
	NetworkResult    = containerapi.NetworkResult
)

const (
	DefaultNamespace = containerapi.DefaultNamespace

	TaskStatusUnknown = containerapi.TaskStatusUnknown
	TaskStatusCreated = containerapi.TaskStatusCreated
	TaskStatusRunning = containerapi.TaskStatusRunning
	TaskStatusStopped = containerapi.TaskStatusStopped
	TaskStatusPaused  = containerapi.TaskStatusPaused
)
