package containerd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"syscall"
	"time"

	tasksv1 "github.com/containerd/containerd/api/services/tasks/v1"
	tasktypes "github.com/containerd/containerd/api/types/task"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/memohai/memoh/internal/config"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrTaskStopTimeout = errors.New("timeout waiting for task to stop")
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

type Service interface {
	PullImage(ctx context.Context, ref string, opts *PullImageOptions) (ImageInfo, error)
	GetImage(ctx context.Context, ref string) (ImageInfo, error)
	ListImages(ctx context.Context) ([]ImageInfo, error)
	DeleteImage(ctx context.Context, ref string, opts *DeleteImageOptions) error
	// ResolveRemoteDigest fetches only the manifest digest from the registry
	// without downloading any layers. Returns ErrNotSupported on backends that
	// have no concept of a remote registry (e.g. Apple Virtualization).
	ResolveRemoteDigest(ctx context.Context, ref string) (string, error)

	CreateContainer(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error)
	GetContainer(ctx context.Context, id string) (ContainerInfo, error)
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
	DeleteContainer(ctx context.Context, id string, opts *DeleteContainerOptions) error
	ListContainersByLabel(ctx context.Context, key, value string) ([]ContainerInfo, error)

	StartContainer(ctx context.Context, containerID string, opts *StartTaskOptions) error
	StopContainer(ctx context.Context, containerID string, opts *StopTaskOptions) error
	DeleteTask(ctx context.Context, containerID string, opts *DeleteTaskOptions) error
	GetTaskInfo(ctx context.Context, containerID string) (TaskInfo, error)
	ListTasks(ctx context.Context, opts *ListTasksOptions) ([]TaskInfo, error)
	SetupNetwork(ctx context.Context, req NetworkSetupRequest) (NetworkResult, error)
	RemoveNetwork(ctx context.Context, req NetworkSetupRequest) error

	CommitSnapshot(ctx context.Context, snapshotter, name, key string) error
	ListSnapshots(ctx context.Context, snapshotter string) ([]SnapshotInfo, error)
	PrepareSnapshot(ctx context.Context, snapshotter, key, parent string) error
	CreateContainerFromSnapshot(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error)
	SnapshotMounts(ctx context.Context, snapshotter, key string) ([]MountInfo, error)
}

type DefaultService struct {
	client    *containerd.Client
	namespace string
	logger    *slog.Logger
}

func NewDefaultService(log *slog.Logger, client *containerd.Client, cfg config.Config) *DefaultService {
	namespace := cfg.Containerd.Namespace
	if namespace == "" {
		namespace = DefaultNamespace
	}
	return &DefaultService{
		client:    client,
		namespace: namespace,
		logger:    log.With(slog.String("service", "containerd")),
	}
}

func (s *DefaultService) PullImage(ctx context.Context, ref string, opts *PullImageOptions) (ImageInfo, error) {
	if ref == "" {
		return ImageInfo{}, ErrInvalidArgument
	}
	ref = config.NormalizeImageRef(ref)

	ctx = s.withNamespace(ctx)
	pullOpts := []containerd.RemoteOpt{}
	if opts == nil || opts.Unpack {
		pullOpts = append(pullOpts, containerd.WithPullUnpack)
	}
	if opts != nil && opts.Snapshotter != "" {
		pullOpts = append(pullOpts, containerd.WithPullSnapshotter(opts.Snapshotter))
	}

	// When OnProgress is set, poll content store for active download statuses.
	if opts != nil && opts.OnProgress != nil {
		stop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			cs := s.client.ContentStore()
			for {
				select {
				case <-stop:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					statuses, err := cs.ListStatuses(ctx)
					if err != nil {
						continue
					}
					layers := make([]LayerStatus, len(statuses))
					for i, st := range statuses {
						layers[i] = LayerStatus{
							Ref:    st.Ref,
							Offset: st.Offset,
							Total:  st.Total,
						}
					}
					opts.OnProgress(PullProgress{Layers: layers})
				}
			}
		}()
		defer close(stop)
	}

	img, err := s.client.Pull(ctx, ref, pullOpts...)
	if err != nil {
		return ImageInfo{}, err
	}
	return toImageInfo(img), nil
}

func (s *DefaultService) GetImage(ctx context.Context, ref string) (ImageInfo, error) {
	if ref == "" {
		return ImageInfo{}, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	img, err := s.getImageWithFallback(ctx, ref)
	if err != nil {
		return ImageInfo{}, err
	}
	return toImageInfo(img), nil
}

func (s *DefaultService) ListImages(ctx context.Context) ([]ImageInfo, error) {
	ctx = s.withNamespace(ctx)
	imgs, err := s.client.ListImages(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ImageInfo, len(imgs))
	for i, img := range imgs {
		result[i] = toImageInfo(img)
	}
	return result, nil
}

func (s *DefaultService) DeleteImage(ctx context.Context, ref string, opts *DeleteImageOptions) error {
	if ref == "" {
		return ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	deleteOpts := []images.DeleteOpt{}
	if opts != nil && opts.Synchronous {
		deleteOpts = append(deleteOpts, images.SynchronousDelete())
	}
	return s.client.ImageService().Delete(ctx, ref, deleteOpts...)
}

func specOptsFromSpec(spec ContainerSpec) []oci.SpecOpts {
	var opts []oci.SpecOpts

	if len(spec.Cmd) > 0 {
		opts = append(opts, oci.WithProcessArgs(spec.Cmd...))
	}
	if len(spec.Env) > 0 {
		opts = append(opts, oci.WithEnv(spec.Env))
	}
	if spec.WorkDir != "" {
		opts = append(opts, oci.WithProcessCwd(spec.WorkDir))
	}
	if spec.User != "" {
		opts = append(opts, oci.WithUser(spec.User))
	}
	if spec.TTY {
		opts = append(opts, oci.WithTTY)
	}
	if len(spec.Mounts) > 0 {
		mounts := make([]specs.Mount, len(spec.Mounts))
		for i, m := range spec.Mounts {
			mounts[i] = specs.Mount{
				Destination: m.Destination,
				Type:        m.Type,
				Source:      m.Source,
				Options:     m.Options,
			}
		}
		opts = append(opts, oci.WithMounts(mounts))
	}

	return opts
}

func (s *DefaultService) CreateContainer(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error) {
	if req.ID == "" || req.ImageRef == "" {
		return ContainerInfo{}, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	ctx, done, err := s.client.WithLease(ctx)
	if err != nil {
		return ContainerInfo{}, err
	}
	defer func() { _ = done(ctx) }()
	image, err := s.getImageWithFallback(ctx, req.ImageRef)
	if err != nil {
		pullOpts := &PullImageOptions{
			Unpack:      true,
			Snapshotter: req.Snapshotter,
		}
		_, err = s.PullImage(ctx, req.ImageRef, pullOpts)
		if err != nil {
			return ContainerInfo{}, err
		}
		image, err = s.getImageWithFallback(ctx, req.ImageRef)
		if err != nil {
			return ContainerInfo{}, err
		}
	}
	snapshotID := req.SnapshotID
	if snapshotID == "" {
		snapshotID = req.ID
	}

	specOpts := []oci.SpecOpts{
		oci.WithDefaultSpecForPlatform("linux/" + runtime.GOARCH),
		oci.WithImageConfig(image),
	}
	specOpts = append(specOpts, specOptsFromSpec(req.Spec)...)

	containerOpts := []containerd.NewContainerOpts{
		containerd.WithImage(image),
	}
	if req.Snapshotter != "" {
		containerOpts = append(containerOpts, containerd.WithSnapshotter(req.Snapshotter))
	}
	if req.Snapshotter != "" {
		parent, err := s.snapshotParentFromLayers(ctx, image)
		if err != nil {
			return ContainerInfo{}, err
		}
		ok, err := s.snapshotExists(ctx, req.Snapshotter, parent)
		if err != nil {
			return ContainerInfo{}, err
		}
		if !ok {
			return ContainerInfo{}, fmt.Errorf("parent snapshot %s does not exist", parent)
		}
		if err := s.prepareSnapshot(ctx, req.Snapshotter, snapshotID, parent); err != nil {
			return ContainerInfo{}, err
		}
		containerOpts = append(containerOpts, containerd.WithSnapshot(snapshotID))
	} else {
		containerOpts = append(containerOpts, containerd.WithNewSnapshot(snapshotID, image))
	}
	containerOpts = append(containerOpts, containerd.WithNewSpec(specOpts...))
	runtimeName := "io.containerd.runc.v2"
	containerOpts = append(containerOpts, containerd.WithRuntime(runtimeName, nil))
	if len(req.Labels) > 0 {
		containerOpts = append(containerOpts, containerd.WithContainerLabels(req.Labels))
	}

	ctrObj, err := s.client.NewContainer(ctx, req.ID, containerOpts...)
	if err != nil {
		return ContainerInfo{}, err
	}
	return toContainerInfo(ctx, ctrObj)
}

func (*DefaultService) snapshotParentFromLayers(ctx context.Context, image containerd.Image) (string, error) {
	diffIDs, err := image.RootFS(ctx)
	if err != nil {
		return "", fmt.Errorf("read image rootfs: %w", err)
	}
	if len(diffIDs) == 0 {
		return "", errors.New("image has no layers")
	}
	chainIDs := identity.ChainIDs(diffIDs)
	return chainIDs[len(chainIDs)-1].String(), nil
}

func (s *DefaultService) snapshotExists(ctx context.Context, snapshotter, key string) (bool, error) {
	if snapshotter == "" || key == "" {
		return false, ErrInvalidArgument
	}
	_, err := s.client.SnapshotService(snapshotter).Stat(ctx, key)
	if err == nil {
		return true, nil
	}
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func (s *DefaultService) prepareSnapshot(ctx context.Context, snapshotter, key, parent string) error {
	if snapshotter == "" || key == "" || parent == "" {
		return ErrInvalidArgument
	}
	sn := s.client.SnapshotService(snapshotter)
	if _, err := sn.Stat(ctx, key); err == nil {
		if err := sn.Remove(ctx, key); err != nil {
			return err
		}
	} else if !errdefs.IsNotFound(err) {
		return err
	}
	_, err := sn.Prepare(ctx, key, parent)
	return err
}

func (s *DefaultService) getImageWithFallback(ctx context.Context, ref string) (containerd.Image, error) {
	image, err := s.client.GetImage(ctx, ref)
	if err == nil {
		return image, nil
	}
	// Official Docker Hub images (e.g. "nginx:latest") may be stored under
	// either "docker.io/library/nginx:latest" or the short form. Try both.
	if strings.HasPrefix(ref, "docker.io/library/") {
		short := strings.TrimPrefix(ref, "docker.io/library/")
		if img, altErr := s.client.GetImage(ctx, short); altErr == nil {
			return img, nil
		}
	}
	return nil, err
}

func (s *DefaultService) GetContainer(ctx context.Context, id string) (ContainerInfo, error) {
	if id == "" {
		return ContainerInfo{}, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	ctrObj, err := s.client.LoadContainer(ctx, id)
	if err != nil {
		return ContainerInfo{}, err
	}
	return toContainerInfo(ctx, ctrObj)
}

func (s *DefaultService) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	ctx = s.withNamespace(ctx)
	ctrs, err := s.client.Containers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ContainerInfo, 0, len(ctrs))
	for _, c := range ctrs {
		info, err := toContainerInfo(ctx, c)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}

func (s *DefaultService) DeleteContainer(ctx context.Context, id string, opts *DeleteContainerOptions) error {
	if id == "" {
		return ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	container, err := s.client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	// A stopped task still holds an entry in containerd; container.Delete fails
	// with FAILED_PRECONDITION if any task entry exists. Delete it first.
	if task, err := container.Task(ctx, nil); err == nil {
		if _, err := task.Delete(ctx, containerd.WithProcessKill); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	} else if !errdefs.IsNotFound(err) {
		return err
	}

	deleteOpts := []containerd.DeleteOpts{}
	cleanupSnapshot := true
	if opts != nil {
		cleanupSnapshot = opts.CleanupSnapshot
	}
	if cleanupSnapshot {
		deleteOpts = append(deleteOpts, containerd.WithSnapshotCleanup)
	}

	return container.Delete(ctx, deleteOpts...)
}

func (s *DefaultService) StartContainer(ctx context.Context, containerID string, _ *StartTaskOptions) error {
	if containerID == "" {
		return ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	container, err := s.client.LoadContainer(ctx, containerID)
	if err != nil {
		return err
	}

	task, err := container.NewTask(ctx, cio.NullIO)
	if err != nil {
		return err
	}
	return task.Start(ctx)
}

func (s *DefaultService) getTask(ctx context.Context, containerID string) (containerd.Task, error) {
	if containerID == "" {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	container, err := s.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return container.Task(ctx, nil)
}

func (s *DefaultService) GetTaskInfo(ctx context.Context, containerID string) (TaskInfo, error) {
	task, err := s.getTask(ctx, containerID)
	if err != nil {
		return TaskInfo{}, err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return TaskInfo{}, err
	}
	return TaskInfo{
		ContainerID: containerID,
		ID:          task.ID(),
		PID:         task.Pid(),
		Status:      convertTaskStatus(status.Status),
		ExitCode:    status.ExitStatus,
	}, nil
}

func (s *DefaultService) ListTasks(ctx context.Context, opts *ListTasksOptions) ([]TaskInfo, error) {
	ctx = s.withNamespace(ctx)
	request := &tasksv1.ListTasksRequest{}
	if opts != nil {
		request.Filter = opts.Filter
	}

	response, err := s.client.TaskService().List(ctx, request)
	if err != nil {
		return nil, err
	}

	tasks := make([]TaskInfo, 0, len(response.Tasks))
	for _, task := range response.Tasks {
		tasks = append(tasks, TaskInfo{
			ContainerID: task.ContainerID,
			ID:          task.ID,
			PID:         task.Pid,
			Status:      convertContainerdTaskStatus(task.Status),
			ExitCode:    task.ExitStatus,
		})
	}

	return tasks, nil
}

func (s *DefaultService) StopContainer(ctx context.Context, containerID string, opts *StopTaskOptions) error {
	if containerID == "" {
		return ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	task, err := s.getTask(ctx, containerID)
	if err != nil {
		return err
	}

	signal := syscall.SIGTERM
	timeout := 10 * time.Second
	force := false
	if opts != nil {
		if opts.Signal != 0 {
			signal = opts.Signal
		}
		if opts.Timeout != 0 {
			timeout = opts.Timeout
		}
		force = opts.Force
	}

	if err := task.Kill(ctx, signal); err != nil {
		return err
	}

	statusC, err := task.Wait(ctx)
	if err != nil {
		return err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-statusC:
		return nil
	case <-timer.C:
		if force {
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				return fmt.Errorf("force kill failed: %w", err)
			}
			<-statusC
			return nil
		}
		return ErrTaskStopTimeout
	}
}

func (s *DefaultService) DeleteTask(ctx context.Context, containerID string, opts *DeleteTaskOptions) error {
	if containerID == "" {
		return ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	task, err := s.getTask(ctx, containerID)
	if err != nil {
		return err
	}

	if opts != nil && opts.Force {
		// Kill and wait for exit before deleting; containerd rejects Delete on a
		// still-running process even when force is requested.
		_ = task.Kill(ctx, syscall.SIGKILL)
		if statusC, waitErr := task.Wait(ctx); waitErr == nil {
			waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			select {
			case <-statusC:
			case <-waitCtx.Done():
			}
		}
	}

	_, err = task.Delete(ctx)
	return err
}

func (s *DefaultService) ListContainersByLabel(ctx context.Context, key, value string) ([]ContainerInfo, error) {
	if key == "" {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	containers, err := s.client.Containers(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]ContainerInfo, 0, len(containers))
	for _, container := range containers {
		ci, err := toContainerInfo(ctx, container)
		if err != nil {
			return nil, err
		}
		if labelValue, ok := ci.Labels[key]; ok && (value == "" || value == labelValue) {
			filtered = append(filtered, ci)
		}
	}
	return filtered, nil
}

func (s *DefaultService) CommitSnapshot(ctx context.Context, snapshotter, name, key string) error {
	if snapshotter == "" || name == "" || key == "" {
		return ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	return s.client.SnapshotService(snapshotter).Commit(ctx, name, key)
}

func (s *DefaultService) ListSnapshots(ctx context.Context, snapshotter string) ([]SnapshotInfo, error) {
	if snapshotter == "" {
		return nil, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	var infos []SnapshotInfo
	if err := s.client.SnapshotService(snapshotter).Walk(ctx, func(_ context.Context, info snapshots.Info) error {
		infos = append(infos, SnapshotInfo{
			Name:    info.Name,
			Parent:  info.Parent,
			Kind:    info.Kind.String(),
			Created: info.Created,
			Updated: info.Updated,
			Labels:  info.Labels,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return infos, nil
}

func (s *DefaultService) PrepareSnapshot(ctx context.Context, snapshotter, key, parent string) error {
	if snapshotter == "" || key == "" || parent == "" {
		return ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	_, err := s.client.SnapshotService(snapshotter).Prepare(ctx, key, parent)
	return err
}

func (s *DefaultService) CreateContainerFromSnapshot(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error) {
	if req.ID == "" || req.SnapshotID == "" {
		return ContainerInfo{}, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)

	imageRef := req.ImageRef
	if imageRef == "" {
		return ContainerInfo{}, ErrInvalidArgument
	}

	image, err := s.getImageWithFallback(ctx, imageRef)
	if err != nil {
		_, pullErr := s.PullImage(ctx, imageRef, &PullImageOptions{
			Unpack:      true,
			Snapshotter: req.Snapshotter,
		})
		if pullErr != nil {
			return ContainerInfo{}, pullErr
		}
		image, err = s.getImageWithFallback(ctx, imageRef)
		if err != nil {
			return ContainerInfo{}, err
		}
	}

	specOpts := []oci.SpecOpts{
		oci.WithDefaultSpecForPlatform("linux/" + runtime.GOARCH),
		oci.WithImageConfig(image),
	}
	specOpts = append(specOpts, specOptsFromSpec(req.Spec)...)

	containerOpts := []containerd.NewContainerOpts{
		containerd.WithImage(image),
	}
	if req.Snapshotter != "" {
		containerOpts = append(containerOpts, containerd.WithSnapshotter(req.Snapshotter))
	}
	containerOpts = append(containerOpts,
		containerd.WithSnapshot(req.SnapshotID),
		containerd.WithNewSpec(specOpts...),
	)
	if len(req.Labels) > 0 {
		containerOpts = append(containerOpts, containerd.WithContainerLabels(req.Labels))
	}

	runtimeName := "io.containerd.runc.v2"
	containerOpts = append(containerOpts, containerd.WithRuntime(runtimeName, nil))

	ctrObj, err := s.client.NewContainer(ctx, req.ID, containerOpts...)
	if err != nil {
		return ContainerInfo{}, err
	}
	return toContainerInfo(ctx, ctrObj)
}

func (s *DefaultService) SnapshotMounts(ctx context.Context, snapshotter, key string) ([]MountInfo, error) {
	if snapshotter == "" || key == "" {
		return nil, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	mounts, err := s.client.SnapshotService(snapshotter).Mounts(ctx, key)
	if err != nil {
		return nil, err
	}
	result := make([]MountInfo, len(mounts))
	for i, m := range mounts {
		result[i] = MountInfo{
			Type:    m.Type,
			Source:  m.Source,
			Options: m.Options,
		}
	}
	return result, nil
}

func (s *DefaultService) SetupNetwork(ctx context.Context, req NetworkSetupRequest) (NetworkResult, error) {
	ctx = s.withNamespace(ctx)
	task, err := s.getTask(ctx, req.ContainerID)
	if err != nil {
		return NetworkResult{}, err
	}
	ip, err := setupCNINetwork(ctx, task, req.ContainerID, req.CNIBinDir, req.CNIConfDir)
	if err != nil {
		return NetworkResult{}, err
	}
	return NetworkResult{IP: ip}, nil
}

func (s *DefaultService) RemoveNetwork(ctx context.Context, req NetworkSetupRequest) error {
	ctx = s.withNamespace(ctx)
	task, err := s.getTask(ctx, req.ContainerID)
	if err != nil {
		return err
	}
	return removeCNINetwork(ctx, task, req.ContainerID, req.CNIBinDir, req.CNIConfDir)
}

func (s *DefaultService) withNamespace(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, s.namespace)
}

func (*DefaultService) ResolveRemoteDigest(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", ErrInvalidArgument
	}
	ref = config.NormalizeImageRef(ref)
	resolver := docker.NewResolver(docker.ResolverOptions{
		Hosts: docker.ConfigureDefaultRegistries(),
	})
	_, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	return desc.Digest.String(), nil
}

func toImageInfo(img containerd.Image) ImageInfo {
	return ImageInfo{
		Name: img.Name(),
		ID:   img.Target().Digest.String(),
		Tags: []string{img.Name()},
	}
}

func toContainerInfo(ctx context.Context, c containerd.Container) (ContainerInfo, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return ContainerInfo{}, err
	}
	return ContainerInfo{
		ID:          info.ID,
		Image:       info.Image,
		Labels:      info.Labels,
		Snapshotter: info.Snapshotter,
		SnapshotKey: info.SnapshotKey,
		Runtime:     RuntimeInfo{Name: info.Runtime.Name},
		CreatedAt:   info.CreatedAt,
		UpdatedAt:   info.UpdatedAt,
	}, nil
}

func convertTaskStatus(s containerd.ProcessStatus) TaskStatus {
	switch s {
	case containerd.Running:
		return TaskStatusRunning
	case containerd.Created:
		return TaskStatusCreated
	case containerd.Stopped:
		return TaskStatusStopped
	case containerd.Paused, containerd.Pausing:
		return TaskStatusPaused
	default:
		return TaskStatusUnknown
	}
}

func convertContainerdTaskStatus(s tasktypes.Status) TaskStatus {
	switch s {
	case tasktypes.Status_RUNNING:
		return TaskStatusRunning
	case tasktypes.Status_CREATED:
		return TaskStatusCreated
	case tasktypes.Status_STOPPED:
		return TaskStatusStopped
	case tasktypes.Status_PAUSED, tasktypes.Status_PAUSING:
		return TaskStatusPaused
	default:
		return TaskStatusUnknown
	}
}
