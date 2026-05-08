package container_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/orchestrationenv"
	envcontainer "github.com/memohai/memoh/internal/orchestrationenv/backend/container"
)

// fakeRuntime records every call the backend issues so tests can
// assert the exact shape and order of runtime interactions without
// pulling in containerd. The "fail next" hooks let tests inject
// targeted failures without rebuilding the whole struct.
type fakeRuntime struct {
	mu sync.Mutex

	pulled      []string
	createCalls []ctr.CreateContainerRequest
	startCalls  []string
	stopCalls   []string
	deleteCalls []string
	commitCalls []ctr.CommitSnapshotRequest

	failPull     error
	failCreate   error
	failStart    error
	failStop     error
	failDelete   error
	failSnapshot error
}

func (f *fakeRuntime) PullImage(_ context.Context, ref string, _ *ctr.PullImageOptions) (ctr.ImageInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pulled = append(f.pulled, ref)
	if f.failPull != nil {
		return ctr.ImageInfo{}, f.failPull
	}
	return ctr.ImageInfo{Name: ref, ID: "img-" + ref}, nil
}

func (f *fakeRuntime) CreateContainer(_ context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls = append(f.createCalls, req)
	if f.failCreate != nil {
		return ctr.ContainerInfo{}, f.failCreate
	}
	return ctr.ContainerInfo{ID: req.ID, Image: req.ImageRef, Labels: req.Labels}, nil
}

func (f *fakeRuntime) StartContainer(_ context.Context, id string, _ *ctr.StartTaskOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, id)
	return f.failStart
}

func (f *fakeRuntime) StopContainer(_ context.Context, id string, _ *ctr.StopTaskOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls = append(f.stopCalls, id)
	return f.failStop
}

func (f *fakeRuntime) DeleteContainer(_ context.Context, id string, _ *ctr.DeleteContainerOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, id)
	return f.failDelete
}

func (f *fakeRuntime) CommitSnapshot(_ context.Context, req ctr.CommitSnapshotRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitCalls = append(f.commitCalls, req)
	return f.failSnapshot
}

func TestBackendKindIsContainer(t *testing.T) {
	b, err := envcontainer.New(&fakeRuntime{}, envcontainer.Options{})
	require.NoError(t, err)
	require.Equal(t, orchestrationenv.KindContainer, b.Kind())
}

func TestBackendAllocatePullsCreatesAndStarts(t *testing.T) {
	rt := &fakeRuntime{}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	res, err := b.Allocate(context.Background(), orchestrationenv.AllocateRequest{
		ResourceID:   "res-1",
		ResourceKind: orchestrationenv.KindContainer,
		ResourceName: "alpine-default",
		ResourceConfig: map[string]any{
			"image":   "alpine:3.20",
			"workdir": "/work",
			"env":     []any{"FOO=bar"},
		},
		SessionID: "sess-1",
		TenantID:  "tenant-A",
		RunID:     "run-1",
		TaskID:    "task-1",
		AttemptID: "att-1",
	})
	require.NoError(t, err)
	require.Len(t, rt.pulled, 1)
	require.Equal(t, "alpine:3.20", rt.pulled[0])
	require.Len(t, rt.createCalls, 1)
	require.Equal(t, "envs-sess-1", rt.createCalls[0].ID)
	require.Equal(t, "alpine:3.20", rt.createCalls[0].ImageRef)
	require.Equal(t, "/work", rt.createCalls[0].Spec.WorkDir)
	require.Equal(t, []string{"FOO=bar"}, rt.createCalls[0].Spec.Env)
	require.Equal(t, "sess-1", rt.createCalls[0].Labels["memoh.orchestration_env.session_id"])
	require.Equal(t, "tenant-A", rt.createCalls[0].Labels["memoh.orchestration_env.tenant_id"])
	require.Equal(t, "run-1", rt.createCalls[0].Labels["memoh.orchestration_env.run_id"])
	require.Len(t, rt.startCalls, 1)
	require.Equal(t, "envs-sess-1", rt.startCalls[0])

	require.Equal(t, "container", res.RuntimeHandle["backend"])
	require.Equal(t, "envs-sess-1", res.RuntimeHandle["container_id"])
	require.Equal(t, "alpine:3.20", res.RuntimeHandle["image_ref"])
	require.Equal(t, "envs-rw-sess-1", res.RuntimeHandle["storage_key"])
}

func TestBackendAllocateRejectsMissingImage(t *testing.T) {
	b, err := envcontainer.New(&fakeRuntime{}, envcontainer.Options{})
	require.NoError(t, err)

	_, err = b.Allocate(context.Background(), orchestrationenv.AllocateRequest{
		SessionID:      "sess-1",
		ResourceName:   "no-image",
		ResourceConfig: map[string]any{},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing image")
}

func TestBackendAllocateCleansUpAfterStartFailure(t *testing.T) {
	rt := &fakeRuntime{failStart: errors.New("boom")}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	_, err = b.Allocate(context.Background(), orchestrationenv.AllocateRequest{
		SessionID:      "sess-1",
		ResourceName:   "r",
		ResourceConfig: map[string]any{"image": "alpine:3.20"},
	})
	require.Error(t, err)
	require.Equal(t, []string{"envs-sess-1"}, rt.deleteCalls,
		"start failure must trigger cleanup deletion")
}

func TestBackendSnapshotCommitsAndReturnsRef(t *testing.T) {
	rt := &fakeRuntime{}
	b, err := envcontainer.New(rt, envcontainer.Options{SnapshotDriver: "overlayfs"})
	require.NoError(t, err)

	res, err := b.Snapshot(context.Background(), orchestrationenv.SnapshotRequestBackend{
		SessionID: "sess-1",
		RuntimeHandle: map[string]any{
			"container_id": "envs-sess-1",
			"storage_key":  "envs-rw-sess-1",
		},
		Kind:        orchestrationenv.SnapshotKindPreAction,
		EffectClass: orchestrationenv.EffectClassEnvLocalMutation,
	})
	require.NoError(t, err)
	require.Len(t, rt.commitCalls, 1)
	require.Equal(t, "overlayfs", rt.commitCalls[0].Source.Driver)
	require.Equal(t, "envs-rw-sess-1", rt.commitCalls[0].Source.Key)
	require.Equal(t, "container", res.RuntimeRef["backend"])
	require.Contains(t, res.RuntimeRef["snapshot_key"].(string), "envs-snap-sess-1-pre_action-")
	require.NotEmpty(t, res.Digest)
}

func TestBackendSnapshotUnsupportedReturnsRefWithFlag(t *testing.T) {
	rt := &fakeRuntime{failSnapshot: ctr.ErrNotSupported}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	res, err := b.Snapshot(context.Background(), orchestrationenv.SnapshotRequestBackend{
		SessionID: "sess-1",
		RuntimeHandle: map[string]any{
			"container_id": "envs-sess-1",
			"storage_key":  "envs-rw-sess-1",
		},
		Kind: orchestrationenv.SnapshotKindPostAction,
	})
	require.NoError(t, err)
	require.Equal(t, true, res.RuntimeRef["unsupported"])
	require.Equal(t, "envs-sess-1", res.RuntimeRef["container_id"])
	require.Empty(t, res.Digest)
}

func TestBackendSnapshotErrorPropagates(t *testing.T) {
	rt := &fakeRuntime{failSnapshot: errors.New("disk full")}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	_, err = b.Snapshot(context.Background(), orchestrationenv.SnapshotRequestBackend{
		SessionID: "sess-1",
		RuntimeHandle: map[string]any{
			"container_id": "envs-sess-1",
			"storage_key":  "envs-rw-sess-1",
		},
		Kind: orchestrationenv.SnapshotKindPostAction,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "commit snapshot")
}

func TestBackendReleaseStopsAndDeletes(t *testing.T) {
	rt := &fakeRuntime{}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	require.NoError(t, b.Release(context.Background(), orchestrationenv.ReleaseRequestBackend{
		SessionID:     "sess-1",
		RuntimeHandle: map[string]any{"container_id": "envs-sess-1"},
		Reason:        "test_done",
	}))
	require.Equal(t, []string{"envs-sess-1"}, rt.stopCalls)
	require.Equal(t, []string{"envs-sess-1"}, rt.deleteCalls)
}

func TestBackendReleaseToleratesStopFailure(t *testing.T) {
	rt := &fakeRuntime{failStop: errors.New("already stopped")}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	require.NoError(t, b.Release(context.Background(), orchestrationenv.ReleaseRequestBackend{
		SessionID:     "sess-1",
		RuntimeHandle: map[string]any{"container_id": "envs-sess-1"},
	}))
	require.Equal(t, []string{"envs-sess-1"}, rt.deleteCalls,
		"stop failure must not prevent delete")
}

func TestBackendReleaseSurfacesDeleteFailure(t *testing.T) {
	rt := &fakeRuntime{failDelete: errors.New("storage busy")}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	err = b.Release(context.Background(), orchestrationenv.ReleaseRequestBackend{
		SessionID:     "sess-1",
		RuntimeHandle: map[string]any{"container_id": "envs-sess-1"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "delete container")
}

func TestBackendReleaseToleratesNotFound(t *testing.T) {
	rt := &fakeRuntime{failStop: ctr.ErrNotFound, failDelete: ctr.ErrNotFound}
	b, err := envcontainer.New(rt, envcontainer.Options{})
	require.NoError(t, err)

	require.NoError(t, b.Release(context.Background(), orchestrationenv.ReleaseRequestBackend{
		SessionID:     "sess-1",
		RuntimeHandle: map[string]any{"container_id": "envs-sess-1"},
	}))
}

func TestBackendNeverPullPolicy(t *testing.T) {
	rt := &fakeRuntime{}
	b, err := envcontainer.New(rt, envcontainer.Options{ImagePullPolicy: "never"})
	require.NoError(t, err)

	_, err = b.Allocate(context.Background(), orchestrationenv.AllocateRequest{
		SessionID:      "sess-1",
		ResourceName:   "r",
		ResourceConfig: map[string]any{"image": "alpine:3.20"},
	})
	require.NoError(t, err)
	require.Empty(t, rt.pulled, "never pull policy must skip PullImage")
}
