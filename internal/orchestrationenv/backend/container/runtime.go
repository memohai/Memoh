package container

import (
	"context"
	"errors"
	"syscall"
	"time"

	ctr "github.com/memohai/memoh/internal/container"
)

// Runtime is the small subset of internal/container.Service that the
// container env backend needs. Defining it here, rather than
// importing the full container.Service, keeps the backend testable
// with a fake and makes the dependency surface explicit.
//
// CommitSnapshot is optional; backends that lack snapshot support
// should return container.ErrNotSupported and the env manager will
// surface a runtime-ref-only result without bytes.
type Runtime interface {
	PullImage(ctx context.Context, ref string, opts *ctr.PullImageOptions) (ctr.ImageInfo, error)
	CreateContainer(ctx context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error)
	StartContainer(ctx context.Context, containerID string, opts *ctr.StartTaskOptions) error
	StopContainer(ctx context.Context, containerID string, opts *ctr.StopTaskOptions) error
	DeleteContainer(ctx context.Context, id string, opts *ctr.DeleteContainerOptions) error
	CommitSnapshot(ctx context.Context, req ctr.CommitSnapshotRequest) error
}

// Options configure backend behaviour. Sensible defaults are
// applied in New so callers usually only set the runtime.
type Options struct {
	// ImagePullPolicy chooses whether Allocate pulls the image before
	// CreateContainer. Mirrors the container service's policy strings
	// ("always", "if_not_present", "never"). Defaults to
	// "if_not_present".
	ImagePullPolicy string

	// SnapshotDriver names the snapshot driver the backend asks the
	// runtime to commit into. Empty defaults to whatever the runtime
	// uses for the source storage ref.
	SnapshotDriver string

	// StopTimeout bounds how long Release waits for a graceful stop
	// before forcing termination. Defaults to 30s.
	StopTimeout time.Duration

	// LabelPrefix is added to container labels so operators can
	// distinguish env-managed containers from bot workspaces or
	// other workloads. Defaults to "memoh.orchestration_env".
	LabelPrefix string
}

func (o Options) withDefaults() Options {
	if o.ImagePullPolicy == "" {
		o.ImagePullPolicy = "if_not_present"
	}
	if o.StopTimeout == 0 {
		o.StopTimeout = 30 * time.Second
	}
	if o.LabelPrefix == "" {
		o.LabelPrefix = "memoh.orchestration_env"
	}
	return o
}

// stopSignal returns the default graceful-stop signal. Hardcoded
// here so the backend stays self-contained — callers that need a
// different signal can wrap the Runtime.
func stopSignal() syscall.Signal {
	return syscall.SIGTERM
}

// errSnapshotUnsupported is returned (wrapped) when the runtime
// cannot snapshot. Callers branch on errors.Is to decide whether to
// surface or to record an unsupported snapshot row.
var errSnapshotUnsupported = errors.New("container: snapshot not supported")
