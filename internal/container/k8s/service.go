package k8s

import (
	"context"

	"github.com/memohai/memoh/internal/config"
	containerapi "github.com/memohai/memoh/internal/container"
)

type Service struct {
	cfg config.Config
}

func NewService(cfg config.Config) *Service {
	return &Service{cfg: cfg}
}

func (*Service) PullImage(context.Context, string, *containerapi.PullImageOptions) (containerapi.ImageInfo, error) {
	return containerapi.ImageInfo{}, containerapi.ErrNotSupported
}

func (*Service) GetImage(context.Context, string) (containerapi.ImageInfo, error) {
	return containerapi.ImageInfo{}, containerapi.ErrNotSupported
}

func (*Service) ListImages(context.Context) ([]containerapi.ImageInfo, error) {
	return nil, containerapi.ErrNotSupported
}

func (*Service) DeleteImage(context.Context, string, *containerapi.DeleteImageOptions) error {
	return containerapi.ErrNotSupported
}

func (*Service) ResolveRemoteDigest(context.Context, string) (string, error) {
	return "", containerapi.ErrNotSupported
}

func (*Service) CreateContainer(context.Context, containerapi.CreateContainerRequest) (containerapi.ContainerInfo, error) {
	return containerapi.ContainerInfo{}, containerapi.ErrNotSupported
}

func (*Service) GetContainer(context.Context, string) (containerapi.ContainerInfo, error) {
	return containerapi.ContainerInfo{}, containerapi.ErrNotSupported
}

func (*Service) ListContainers(context.Context) ([]containerapi.ContainerInfo, error) {
	return nil, containerapi.ErrNotSupported
}

func (*Service) DeleteContainer(context.Context, string, *containerapi.DeleteContainerOptions) error {
	return containerapi.ErrNotSupported
}

func (*Service) ListContainersByLabel(context.Context, string, string) ([]containerapi.ContainerInfo, error) {
	return nil, containerapi.ErrNotSupported
}

func (*Service) CreateContainerFromSnapshot(context.Context, containerapi.CreateContainerRequest) (containerapi.ContainerInfo, error) {
	return containerapi.ContainerInfo{}, containerapi.ErrNotSupported
}

func (*Service) StartContainer(context.Context, string, *containerapi.StartTaskOptions) error {
	return containerapi.ErrNotSupported
}

func (*Service) StopContainer(context.Context, string, *containerapi.StopTaskOptions) error {
	return containerapi.ErrNotSupported
}

func (*Service) DeleteTask(context.Context, string, *containerapi.DeleteTaskOptions) error {
	return containerapi.ErrNotSupported
}

func (*Service) GetTaskInfo(context.Context, string) (containerapi.TaskInfo, error) {
	return containerapi.TaskInfo{}, containerapi.ErrNotSupported
}

func (*Service) GetContainerMetrics(context.Context, string) (containerapi.ContainerMetrics, error) {
	return containerapi.ContainerMetrics{}, containerapi.ErrNotSupported
}

func (*Service) ListTasks(context.Context, *containerapi.ListTasksOptions) ([]containerapi.TaskInfo, error) {
	return nil, containerapi.ErrNotSupported
}

func (*Service) SetupNetwork(context.Context, containerapi.NetworkRequest) (containerapi.NetworkResult, error) {
	return containerapi.NetworkResult{}, nil
}

func (*Service) RemoveNetwork(context.Context, containerapi.NetworkRequest) error {
	return nil
}

func (*Service) CheckNetwork(context.Context, containerapi.NetworkRequest) error {
	return nil
}

func (*Service) CommitSnapshot(context.Context, string, string, string) error {
	return containerapi.ErrNotSupported
}

func (*Service) ListSnapshots(context.Context, string) ([]containerapi.SnapshotInfo, error) {
	return nil, containerapi.ErrNotSupported
}

func (*Service) PrepareSnapshot(context.Context, string, string, string) error {
	return containerapi.ErrNotSupported
}

func (*Service) SnapshotUsage(context.Context, string, string) (containerapi.SnapshotUsage, error) {
	return containerapi.SnapshotUsage{}, containerapi.ErrNotSupported
}

func (*Service) SnapshotMounts(context.Context, string, string) ([]containerapi.MountInfo, error) {
	return nil, containerapi.ErrNotSupported
}
