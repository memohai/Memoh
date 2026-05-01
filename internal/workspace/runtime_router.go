package workspace

import (
	"context"
	"strings"

	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type RuntimeRouter struct {
	container runtimeService
	local     *LocalService
}

func NewRuntimeRouter(container runtimeService, local *LocalService) *RuntimeRouter {
	return &RuntimeRouter{container: container, local: local}
}

func (r *RuntimeRouter) LocalEnabled() bool {
	return r.localEnabled()
}

func (r *RuntimeRouter) DefaultLocalWorkspacePath(botID, displayName string) string {
	if !r.localEnabled() {
		return ""
	}
	return r.local.DefaultWorkspacePath(botID, displayName)
}

func (r *RuntimeRouter) localEnabled() bool {
	return r != nil && r.local != nil && r.local.Enabled()
}

func (r *RuntimeRouter) routeByID(id string) runtimeService {
	if r.localEnabled() && strings.HasPrefix(strings.TrimSpace(id), LocalContainerPrefix) {
		return r.local
	}
	return r.container
}

func (r *RuntimeRouter) routeCreate(req ctr.CreateContainerRequest) runtimeService {
	if r.localEnabled() && (strings.HasPrefix(strings.TrimSpace(req.ID), LocalContainerPrefix) || strings.TrimSpace(req.StorageRef.Driver) == localRuntimeName) {
		return r.local
	}
	return r.container
}

func (r *RuntimeRouter) CreateContainer(ctx context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	return r.routeCreate(req).CreateContainer(ctx, req)
}

func (r *RuntimeRouter) GetContainer(ctx context.Context, id string) (ctr.ContainerInfo, error) {
	return r.routeByID(id).GetContainer(ctx, id)
}

func (r *RuntimeRouter) ListContainers(ctx context.Context) ([]ctr.ContainerInfo, error) {
	out, err := r.container.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	if r.localEnabled() {
		localItems, err := r.local.ListContainers(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, localItems...)
	}
	return out, nil
}

func (r *RuntimeRouter) DeleteContainer(ctx context.Context, id string, opts *ctr.DeleteContainerOptions) error {
	return r.routeByID(id).DeleteContainer(ctx, id, opts)
}

func (r *RuntimeRouter) ListContainersByLabel(ctx context.Context, key, value string) ([]ctr.ContainerInfo, error) {
	out, err := r.container.ListContainersByLabel(ctx, key, value)
	if err != nil {
		return nil, err
	}
	if r.localEnabled() {
		localItems, err := r.local.ListContainersByLabel(ctx, key, value)
		if err != nil {
			return nil, err
		}
		out = append(out, localItems...)
	}
	return out, nil
}

func (r *RuntimeRouter) StartContainer(ctx context.Context, containerID string, opts *ctr.StartTaskOptions) error {
	return r.routeByID(containerID).StartContainer(ctx, containerID, opts)
}

func (r *RuntimeRouter) StopContainer(ctx context.Context, containerID string, opts *ctr.StopTaskOptions) error {
	return r.routeByID(containerID).StopContainer(ctx, containerID, opts)
}

func (r *RuntimeRouter) DeleteTask(ctx context.Context, containerID string, opts *ctr.DeleteTaskOptions) error {
	return r.routeByID(containerID).DeleteTask(ctx, containerID, opts)
}

func (r *RuntimeRouter) GetTaskInfo(ctx context.Context, containerID string) (ctr.TaskInfo, error) {
	return r.routeByID(containerID).GetTaskInfo(ctx, containerID)
}

func (r *RuntimeRouter) GetContainerMetrics(ctx context.Context, containerID string) (ctr.ContainerMetrics, error) {
	return r.routeByID(containerID).GetContainerMetrics(ctx, containerID)
}

func (r *RuntimeRouter) ListTasks(ctx context.Context, opts *ctr.ListTasksOptions) ([]ctr.TaskInfo, error) {
	out, err := r.container.ListTasks(ctx, opts)
	if err != nil {
		return nil, err
	}
	if r.localEnabled() {
		localItems, err := r.local.ListTasks(ctx, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, localItems...)
	}
	return out, nil
}

func (r *RuntimeRouter) SetupNetwork(ctx context.Context, req ctr.NetworkRequest) (ctr.NetworkResult, error) {
	return r.routeByID(req.ContainerID).SetupNetwork(ctx, req)
}

func (r *RuntimeRouter) RemoveNetwork(ctx context.Context, req ctr.NetworkRequest) error {
	return r.routeByID(req.ContainerID).RemoveNetwork(ctx, req)
}

func (r *RuntimeRouter) CheckNetwork(ctx context.Context, req ctr.NetworkRequest) error {
	return r.routeByID(req.ContainerID).CheckNetwork(ctx, req)
}

func (r *RuntimeRouter) CommitSnapshot(ctx context.Context, req ctr.CommitSnapshotRequest) error {
	if strings.TrimSpace(req.Source.Driver) == localRuntimeName {
		if !r.localEnabled() {
			return ctr.ErrNotSupported
		}
		return r.local.CommitSnapshot(ctx, req)
	}
	return r.container.CommitSnapshot(ctx, req)
}

func (r *RuntimeRouter) ListSnapshots(ctx context.Context, req ctr.ListSnapshotsRequest) ([]ctr.SnapshotInfo, error) {
	if strings.TrimSpace(req.Driver) == localRuntimeName {
		return nil, ctr.ErrNotSupported
	}
	return r.container.ListSnapshots(ctx, req)
}

func (r *RuntimeRouter) PrepareSnapshot(ctx context.Context, req ctr.PrepareSnapshotRequest) error {
	if strings.TrimSpace(req.Target.Driver) == localRuntimeName {
		if !r.localEnabled() {
			return ctr.ErrNotSupported
		}
		return r.local.PrepareSnapshot(ctx, req)
	}
	return r.container.PrepareSnapshot(ctx, req)
}

func (r *RuntimeRouter) RestoreContainer(ctx context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	return r.routeCreate(req).RestoreContainer(ctx, req)
}

func (r *RuntimeRouter) PullImage(ctx context.Context, ref string, opts *ctr.PullImageOptions) (ctr.ImageInfo, error) {
	imageService, ok := r.container.(ctr.ImageService)
	if !ok {
		return ctr.ImageInfo{}, ctr.ErrNotSupported
	}
	return imageService.PullImage(ctx, ref, opts)
}

func (r *RuntimeRouter) GetImage(ctx context.Context, ref string) (ctr.ImageInfo, error) {
	imageService, ok := r.container.(ctr.ImageService)
	if !ok {
		return ctr.ImageInfo{}, ctr.ErrNotSupported
	}
	return imageService.GetImage(ctx, ref)
}

func (r *RuntimeRouter) ListImages(ctx context.Context) ([]ctr.ImageInfo, error) {
	imageService, ok := r.container.(ctr.ImageService)
	if !ok {
		return nil, ctr.ErrNotSupported
	}
	return imageService.ListImages(ctx)
}

func (r *RuntimeRouter) DeleteImage(ctx context.Context, ref string, opts *ctr.DeleteImageOptions) error {
	imageService, ok := r.container.(ctr.ImageService)
	if !ok {
		return ctr.ErrNotSupported
	}
	return imageService.DeleteImage(ctx, ref, opts)
}

func (r *RuntimeRouter) ResolveRemoteDigest(ctx context.Context, ref string) (string, error) {
	imageService, ok := r.container.(ctr.ImageService)
	if !ok {
		return "", ctr.ErrNotSupported
	}
	return imageService.ResolveRemoteDigest(ctx, ref)
}

func (r *RuntimeRouter) SnapshotMounts(ctx context.Context, snapshotter, key string) ([]ctr.MountInfo, error) {
	if strings.TrimSpace(snapshotter) == localRuntimeName {
		return nil, ctr.ErrNotSupported
	}
	mounter, ok := r.container.(interface {
		SnapshotMounts(context.Context, string, string) ([]ctr.MountInfo, error)
	})
	if !ok {
		return nil, ctr.ErrNotSupported
	}
	return mounter.SnapshotMounts(ctx, snapshotter, key)
}

func (r *RuntimeRouter) MCPClient(ctx context.Context, botID string) (*bridge.Client, error) {
	if r.localEnabled() {
		if _, err := r.local.GetContainer(ctx, LocalContainerPrefix+strings.TrimSpace(botID)); err == nil {
			return r.local.MCPClient(ctx, botID)
		} else if !ctr.IsNotFound(err) {
			return nil, err
		}
	}
	return nil, ctr.ErrNotSupported
}

func (r *RuntimeRouter) WorkspaceInfo(ctx context.Context, botID string) (bridge.WorkspaceInfo, error) {
	if r.localEnabled() {
		if info, err := r.local.WorkspaceInfo(ctx, botID); err == nil {
			return info, nil
		} else if !ctr.IsNotFound(err) {
			return bridge.WorkspaceInfo{}, err
		}
	}
	return bridge.WorkspaceInfo{
		Backend:        bridge.WorkspaceBackendContainer,
		DefaultWorkDir: "/data",
	}, nil
}
