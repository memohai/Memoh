package containerd

import (
	"errors"
	"io"
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

type NetworkSetupRequest struct {
	ContainerID string
	PID         uint32
	CNIBinDir   string
	CNIConfDir  string
}

type ExecTaskRequest struct {
	Args     []string
	Env      []string
	WorkDir  string
	Terminal bool
	UseStdio bool
	FIFODir  string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type ExecTaskSession struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
	Wait   func() (ExecTaskResult, error)
	Close  func() error
}

type ExecTaskResult struct {
	ExitCode uint32
}
