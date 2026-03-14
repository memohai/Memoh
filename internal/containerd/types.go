package containerd

import (
	"errors"
	"time"
)

var ErrNotSupported = errors.New("operation not supported on this backend")

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
	Cmd     []string
	Env     []string
	WorkDir string
	User    string
	Mounts  []MountSpec
	DNS     []string
	TTY     bool
}

type LayerStatus struct {
	Ref    string `json:"ref"`
	Offset int64  `json:"offset"`
	Total  int64  `json:"total"`
}

type PullProgress struct {
	Layers []LayerStatus `json:"layers"`
}

type NetworkSetupRequest struct {
	ContainerID string
	PID         uint32
	CNIBinDir   string
	CNIConfDir  string
}

type NetworkResult struct {
	IP string
}
