package container

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/orchestrationenv"
)

// runtimeHandleKey names the JSON keys the backend stamps into
// orchestration_env_sessions.runtime_handle so future Snapshot /
// Release calls can find their way back to the live container.
const (
	handleKeyBackend     = "backend"
	handleKeyBackendKind = "backend_kind"
	handleKeyContainerID = "container_id"
	handleKeyImageRef    = "image_ref"
	handleKeyStorageKey  = "storage_key"
	handleKeySessionID   = "session_id"
)

// Backend is the orchestrationenv.Backend implementation for
// container env resources. Allocate creates and starts a fresh
// container per session, Snapshot delegates to the runtime, and
// Release stops and deletes the container.
type Backend struct {
	runtime Runtime
	opts    Options
}

// New returns a backend wired to the given runtime. opts are merged
// with defaults so callers can pass a zero value.
func New(runtime Runtime, opts Options) (*Backend, error) {
	if runtime == nil {
		return nil, errors.New("container backend: runtime is required")
	}
	return &Backend{runtime: runtime, opts: opts.withDefaults()}, nil
}

// Kind reports the env kind this backend handles.
func (*Backend) Kind() string {
	return orchestrationenv.KindContainer
}

// Allocate pulls the configured image (subject to the pull policy),
// creates a container, starts it, and returns a runtime handle.
func (b *Backend) Allocate(ctx context.Context, req orchestrationenv.AllocateRequest) (orchestrationenv.AllocateResult, error) {
	imageRef := stringFrom(req.ResourceConfig, "image", "image_ref")
	if imageRef == "" {
		return orchestrationenv.AllocateResult{}, fmt.Errorf("container backend: resource %q config missing image", req.ResourceName)
	}
	storageDriver := stringFrom(req.ResourceConfig, "storage_driver", "snapshotter")

	if b.opts.ImagePullPolicy != "never" {
		if _, err := b.runtime.PullImage(ctx, imageRef, &ctr.PullImageOptions{
			Unpack:        true,
			StorageDriver: storageDriver,
		}); err != nil {
			return orchestrationenv.AllocateResult{}, fmt.Errorf("container backend: pull image %q: %w", imageRef, err)
		}
	}

	containerID := containerIDFor(req.SessionID)
	storageKey := storageKeyFor(req.SessionID)
	storageRef := ctr.StorageRef{
		Driver: storageDriver,
		Key:    storageKey,
	}
	createReq := ctr.CreateContainerRequest{
		ID:              containerID,
		ImageRef:        imageRef,
		ImagePullPolicy: b.opts.ImagePullPolicy,
		StorageRef:      storageRef,
		Labels:          b.labelsFor(req),
		Spec:            specFromConfig(req.ResourceConfig),
	}
	info, err := b.runtime.CreateContainer(ctx, createReq)
	if err != nil {
		return orchestrationenv.AllocateResult{}, fmt.Errorf("container backend: create container: %w", err)
	}

	if err := b.runtime.StartContainer(ctx, info.ID, &ctr.StartTaskOptions{}); err != nil {
		// Best-effort cleanup so a half-created container does not
		// linger and exhaust capacity.
		_ = b.runtime.DeleteContainer(ctx, info.ID, &ctr.DeleteContainerOptions{CleanupSnapshot: true})
		return orchestrationenv.AllocateResult{}, fmt.Errorf("container backend: start container: %w", err)
	}

	handle := map[string]any{
		handleKeyBackend:     "container",
		handleKeyBackendKind: orchestrationenv.KindContainer,
		handleKeyContainerID: info.ID,
		handleKeyImageRef:    imageRef,
		handleKeyStorageKey:  storageKey,
		handleKeySessionID:   req.SessionID,
	}
	return orchestrationenv.AllocateResult{
		RuntimeHandle: handle,
	}, nil
}

// Snapshot commits the container's current state to a snapshot. The
// returned runtime ref carries the snapshot driver + key so verifier
// replay or HITL diff can reattach to the snapshot bytes.
func (b *Backend) Snapshot(ctx context.Context, req orchestrationenv.SnapshotRequestBackend) (orchestrationenv.SnapshotResult, error) {
	containerID := stringFromHandle(req.RuntimeHandle, handleKeyContainerID)
	storageKey := stringFromHandle(req.RuntimeHandle, handleKeyStorageKey)
	if containerID == "" || storageKey == "" {
		return orchestrationenv.SnapshotResult{}, errors.New("container backend: runtime handle missing container_id/storage_key")
	}
	snapshotKey := snapshotKeyFor(req.SessionID, req.Kind)
	commitReq := ctr.CommitSnapshotRequest{
		Source: ctr.StorageRef{Driver: b.opts.SnapshotDriver, Key: storageKey},
		Target: ctr.SnapshotRef{Driver: b.opts.SnapshotDriver, Key: snapshotKey, Kind: "committed"},
	}
	if err := b.runtime.CommitSnapshot(ctx, commitReq); err != nil {
		if errors.Is(err, ctr.ErrNotSupported) || errors.Is(err, errSnapshotUnsupported) {
			return orchestrationenv.SnapshotResult{
				RuntimeRef: map[string]any{
					"backend":      "container",
					"unsupported":  true,
					"container_id": containerID,
					"snapshot_key": snapshotKey,
				},
			}, nil
		}
		return orchestrationenv.SnapshotResult{}, fmt.Errorf("container backend: commit snapshot: %w", err)
	}
	digest := digestFromKey(snapshotKey)
	return orchestrationenv.SnapshotResult{
		RuntimeRef: map[string]any{
			"backend":      "container",
			"container_id": containerID,
			"snapshot_key": snapshotKey,
			"driver":       b.opts.SnapshotDriver,
		},
		Digest: digest,
	}, nil
}

// Release stops and deletes the container. Errors at the stop stage
// are tolerated (the runtime may have already exited); errors at the
// delete stage surface back so the manager can decide.
func (b *Backend) Release(ctx context.Context, req orchestrationenv.ReleaseRequestBackend) error {
	containerID := stringFromHandle(req.RuntimeHandle, handleKeyContainerID)
	if containerID == "" {
		return nil
	}
	if err := b.runtime.StopContainer(ctx, containerID, &ctr.StopTaskOptions{
		Signal:  stopSignal(),
		Timeout: b.opts.StopTimeout,
		Force:   true,
	}); err != nil && !errors.Is(err, ctr.ErrNotFound) {
		// Stop failures are logged via the manager; deletion
		// continues so a stuck container does not leak the slot.
		_ = err
	}
	if err := b.runtime.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{
		CleanupSnapshot: true,
	}); err != nil && !errors.Is(err, ctr.ErrNotFound) {
		return fmt.Errorf("container backend: delete container: %w", err)
	}
	return nil
}

// labelsFor builds the label set the backend stamps onto the
// container so operators can correlate it back to the env session
// without consulting Postgres.
func (b *Backend) labelsFor(req orchestrationenv.AllocateRequest) map[string]string {
	prefix := b.opts.LabelPrefix
	labels := map[string]string{
		prefix + ".session_id":  req.SessionID,
		prefix + ".tenant_id":   req.TenantID,
		prefix + ".resource_id": req.ResourceID,
	}
	if req.RunID != "" {
		labels[prefix+".run_id"] = req.RunID
	}
	if req.TaskID != "" {
		labels[prefix+".task_id"] = req.TaskID
	}
	if req.AttemptID != "" {
		labels[prefix+".attempt_id"] = req.AttemptID
	}
	return labels
}

// containerIDFor turns an env session id into a stable container id.
// Using the env_session_id directly keeps lookups trivial; sticking
// the "envs-" prefix on means operators can spot env-managed
// containers at a glance.
func containerIDFor(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return "envs-" + uuid.NewString()
	}
	return "envs-" + sessionID
}

// storageKeyFor returns the snapshot/storage key the runtime should
// use for the active container. Symmetric with containerIDFor so a
// session always reattaches to the same storage on retry.
func storageKeyFor(sessionID string) string {
	return "envs-rw-" + sessionID
}

// snapshotKeyFor builds a per-snapshot key. Including the snapshot
// kind ('pre_action', 'post_action', ...) keeps multiple snapshots
// per session distinguishable.
func snapshotKeyFor(sessionID, kind string) string {
	if kind == "" {
		kind = "snap"
	}
	return "envs-snap-" + sessionID + "-" + kind + "-" + uuid.NewString()[:8]
}

// digestFromKey hashes the snapshot key so callers always get a
// deterministic content-style identifier even before the runtime
// surfaces real content addressing.
func digestFromKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// stringFrom returns the first non-empty string found under any of
// the given keys in the map. Useful for resource configs that may
// spell the same field multiple ways.
func stringFrom(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func stringFromHandle(handle map[string]any, key string) string {
	if handle == nil {
		return ""
	}
	if v, ok := handle[key].(string); ok {
		return v
	}
	return ""
}

// specFromConfig translates the resource config into a container spec
// the runtime understands. Today only Cmd / Env / WorkDir are honoured;
// Stage 3-E will plumb mounts and network attach as the kernel needs
// them.
func specFromConfig(cfg map[string]any) ctr.ContainerSpec {
	spec := ctr.ContainerSpec{}
	if v, ok := cfg["cmd"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				spec.Cmd = append(spec.Cmd, s)
			}
		}
	}
	if v, ok := cfg["env"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				spec.Env = append(spec.Env, s)
			}
		}
	}
	if v, ok := cfg["workdir"].(string); ok {
		spec.WorkDir = v
	}
	if v, ok := cfg["user"].(string); ok {
		spec.User = v
	}
	return spec
}
