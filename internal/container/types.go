package container

import (
	"errors"
	"time"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotSupported    = errors.New("operation not supported on this backend")
)

type TaskStatus int

const (
	TaskStatusUnknown TaskStatus = iota
	TaskStatusCreated
	TaskStatusRunning
	TaskStatusStopped
	TaskStatusPaused
)

func (s TaskStatus) String() string {
	switch s {
	case TaskStatusCreated:
		return "CREATED"
	case TaskStatusRunning:
		return "RUNNING"
	case TaskStatusStopped:
		return "STOPPED"
	case TaskStatusPaused:
		return "PAUSED"
	default:
		return "UNKNOWN"
	}
}

type ContainerInfo struct {
	ID          string
	Image       string
	Labels      map[string]string
	Snapshotter string
	SnapshotKey string
	Runtime     RuntimeInfo
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type RuntimeInfo struct {
	Name string
}

type ImageInfo struct {
	Name string
	ID   string
	Tags []string
}

type TaskInfo struct {
	ContainerID string
	ID          string
	PID         uint32
	Status      TaskStatus
	ExitCode    uint32
}

type ContainerMetrics struct {
	SampledAt time.Time
	CPU       *CPUMetrics
	Memory    *MemoryMetrics
}

type CPUMetrics struct {
	UsagePercent      float64
	UsageNanoseconds  uint64
	UserNanoseconds   uint64
	KernelNanoseconds uint64
}

type MemoryMetrics struct {
	UsageBytes   uint64
	LimitBytes   uint64
	UsagePercent float64
}

type SnapshotInfo struct {
	Name    string
	Parent  string
	Kind    string
	Created time.Time
	Updated time.Time
	Labels  map[string]string
}

type MountInfo struct {
	Type    string
	Source  string
	Target  string
	Options []string
}

type MountSpec struct {
	Destination string
	Type        string
	Source      string
	Options     []string
}

type ContainerSpec struct {
	Cmd                  []string
	Env                  []string
	WorkDir              string
	User                 string
	Mounts               []MountSpec
	DNS                  []string
	NetworkNamespacePath string
	AddedCapabilities    []string
	// CDIDevices contains fully-qualified CDI device names such as
	// "nvidia.com/gpu=0" or "amd.com/gpu=0".
	CDIDevices []string
	TTY        bool
}

type LayerStatus struct {
	Ref    string `json:"ref"`
	Offset int64  `json:"offset"`
	Total  int64  `json:"total"`
}

type PullProgress struct {
	Layers []LayerStatus `json:"layers"`
}

// NetworkRequest describes the host-side wiring required to attach a container
// task to the default CNI-provided network for basic outbound connectivity.
// It does not describe future provider/overlay networking.
type NetworkRequest struct {
	ContainerID string
	NetNSPath   string
	PID         uint32
	CNIBinDir   string
	CNIConfDir  string
}

// NetworkResult captures the outcome of attaching the container to the default
// container network.
type NetworkResult struct {
	IP string
}
