package containerd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	tasksv1 "github.com/containerd/containerd/api/services/tasks/v1"
	tasktypes "github.com/containerd/containerd/api/types/task"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/memohai/memoh/internal/config"
	"github.com/opencontainers/image-spec/identity"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrTaskStopTimeout = errors.New("timeout waiting for task to stop")
)

type PullImageOptions struct {
	Unpack      bool
	Snapshotter string
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
	UseStdio bool
	Terminal bool
	FIFODir  string
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
	ExecTask(ctx context.Context, containerID string, req ExecTaskRequest) (ExecTaskResult, error)
	ExecTaskStreaming(ctx context.Context, containerID string, req ExecTaskRequest) (*ExecTaskSession, error)

	SetupNetwork(ctx context.Context, req NetworkSetupRequest) error
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

	ctx = s.withNamespace(ctx)
	pullOpts := []containerd.RemoteOpt{}
	if opts == nil || opts.Unpack {
		pullOpts = append(pullOpts, containerd.WithPullUnpack)
	}
	if opts != nil && opts.Snapshotter != "" {
		pullOpts = append(pullOpts, containerd.WithPullSnapshotter(opts.Snapshotter))
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
	defer done(ctx)
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

func (s *DefaultService) snapshotParentFromLayers(ctx context.Context, image containerd.Image) (string, error) {
	diffIDs, err := image.RootFS(ctx)
	if err != nil {
		return "", fmt.Errorf("read image rootfs: %w", err)
	}
	if len(diffIDs) == 0 {
		return "", fmt.Errorf("image has no layers")
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
	if strings.HasPrefix(ref, "docker.io/library/") {
		alt := strings.TrimPrefix(ref, "docker.io/library/")
		image, altErr := s.client.GetImage(ctx, alt)
		if altErr == nil {
			return image, nil
		}
	}
	imgs, listErr := s.client.ListImages(ctx)
	if listErr == nil {
		for _, img := range imgs {
			name := img.Name()
			if name == ref || strings.HasSuffix(ref, "/"+name) || strings.HasSuffix(name, "/"+ref) {
				return img, nil
			}
			if strings.HasPrefix(ref, "docker.io/library/") {
				alt := strings.TrimPrefix(ref, "docker.io/library/")
				if name == alt || strings.HasSuffix(name, "/"+alt) {
					return img, nil
				}
			}
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

func (s *DefaultService) StartContainer(ctx context.Context, containerID string, opts *StartTaskOptions) error {
	if containerID == "" {
		return ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	container, err := s.client.LoadContainer(ctx, containerID)
	if err != nil {
		return err
	}

	var ioCreator cio.Creator
	if opts == nil || !opts.UseStdio {
		ioCreator = cio.NullIO
	} else {
		cioOpts := []cio.Opt{cio.WithStdio}
		if opts.Terminal {
			cioOpts = append(cioOpts, cio.WithTerminal)
		}
		if opts.FIFODir != "" {
			cioOpts = append(cioOpts, cio.WithFIFODir(opts.FIFODir))
		}
		ioCreator = cio.NewCreator(cioOpts...)
	}

	task, err := container.NewTask(ctx, ioCreator)
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
		_ = task.Kill(ctx, syscall.SIGKILL)
	}

	_, err = task.Delete(ctx)
	return err
}

func (s *DefaultService) ExecTask(ctx context.Context, containerID string, req ExecTaskRequest) (ExecTaskResult, error) {
	if containerID == "" || len(req.Args) == 0 {
		return ExecTaskResult{}, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	container, err := s.client.LoadContainer(ctx, containerID)
	if err != nil {
		return ExecTaskResult{}, err
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		return ExecTaskResult{}, err
	}
	if spec.Process == nil {
		spec.Process = &specs.Process{}
	}

	if len(req.Env) > 0 {
		if err := oci.WithEnv(req.Env)(ctx, nil, nil, spec); err != nil {
			return ExecTaskResult{}, err
		}
	}

	spec.Process.Args = req.Args
	if req.WorkDir != "" {
		spec.Process.Cwd = req.WorkDir
	}
	if req.Terminal {
		spec.Process.Terminal = true
	}

	task, err := s.getTask(ctx, containerID)
	if err != nil {
		return ExecTaskResult{}, err
	}

	ioOpts := []cio.Opt{}
	if req.Stdin != nil || req.Stdout != nil || req.Stderr != nil {
		ioOpts = append(ioOpts, cio.WithStreams(req.Stdin, req.Stdout, req.Stderr))
	} else if req.UseStdio {
		ioOpts = append(ioOpts, cio.WithStdio)
	}
	if req.Terminal {
		ioOpts = append(ioOpts, cio.WithTerminal)
	}
	if strings.TrimSpace(req.FIFODir) != "" {
		if err := os.MkdirAll(req.FIFODir, 0o755); err != nil {
			return ExecTaskResult{}, err
		}
		ioOpts = append(ioOpts, cio.WithFIFODir(req.FIFODir))
	}
	ioCreator := cio.NewCreator(ioOpts...)

	execID := fmt.Sprintf("exec-%d", time.Now().UnixNano())
	process, err := task.Exec(ctx, execID, spec.Process, ioCreator)
	if err != nil {
		return ExecTaskResult{}, err
	}
	defer process.Delete(ctx)

	statusC, err := process.Wait(ctx)
	if err != nil {
		return ExecTaskResult{}, err
	}
	if err := process.Start(ctx); err != nil {
		return ExecTaskResult{}, err
	}

	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return ExecTaskResult{}, err
	}

	return ExecTaskResult{ExitCode: code}, nil
}

func (s *DefaultService) ExecTaskStreaming(ctx context.Context, containerID string, req ExecTaskRequest) (*ExecTaskSession, error) {
	if containerID == "" || len(req.Args) == 0 {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	container, err := s.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, err
	}
	if spec.Process == nil {
		spec.Process = &specs.Process{}
	}
	if len(req.Env) > 0 {
		if err := oci.WithEnv(req.Env)(ctx, nil, nil, spec); err != nil {
			return nil, err
		}
	}
	spec.Process.Args = req.Args
	if req.WorkDir != "" {
		spec.Process.Cwd = req.WorkDir
	}
	if req.Terminal {
		spec.Process.Terminal = true
	}

	task, err := s.getTask(ctx, containerID)
	if err != nil {
		return nil, err
	}

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	ioOpts := []cio.Opt{
		cio.WithStreams(stdinR, stdoutW, stderrW),
	}
	if req.Terminal {
		ioOpts = append(ioOpts, cio.WithTerminal)
	}
	fifoDir, err := resolveExecFIFODir(req.FIFODir)
	if err != nil {
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		return nil, err
	}
	ioOpts = append(ioOpts, cio.WithFIFODir(fifoDir))
	ioCreator := cio.NewCreator(ioOpts...)

	execID := fmt.Sprintf("exec-%d", time.Now().UnixNano())
	process, err := task.Exec(ctx, execID, spec.Process, ioCreator)
	if err != nil {
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		return nil, err
	}

	if err := process.Start(ctx); err != nil {
		_, _ = process.Delete(ctx)
		_ = stdinR.Close()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		return nil, err
	}

	wait := func() (ExecTaskResult, error) {
		statusC, err := process.Wait(ctx)
		if err != nil {
			return ExecTaskResult{}, err
		}
		status := <-statusC
		code, _, err := status.Result()
		if err != nil {
			return ExecTaskResult{}, err
		}
		_, _ = process.Delete(ctx)
		_ = stdoutW.Close()
		_ = stderrW.Close()
		return ExecTaskResult{ExitCode: code}, nil
	}

	closeFn := func() error {
		_ = stdinW.Close()
		_ = stdoutR.Close()
		_ = stderrR.Close()
		_ = stdinR.Close()
		_ = stdoutW.Close()
		_ = stderrW.Close()
		_, err := process.Delete(ctx)
		return err
	}

	return &ExecTaskSession{
		Stdin:  stdinW,
		Stdout: stdoutR,
		Stderr: stderrR,
		Wait:   wait,
		Close:  closeFn,
	}, nil
}

func resolveExecFIFODir(preferred string) (string, error) {
	candidates := make([]string, 0, 3)
	if p := strings.TrimSpace(preferred); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, "/var/lib/containerd/memoh-fifo", "/tmp/memoh-containerd-fifo")

	var lastErr error
	for _, dir := range candidates {
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no fifo directory candidate available")
	}
	return "", lastErr
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
	if err := s.client.SnapshotService(snapshotter).Walk(ctx, func(ctx context.Context, info snapshots.Info) error {
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

func (s *DefaultService) SetupNetwork(ctx context.Context, req NetworkSetupRequest) error {
	ctx = s.withNamespace(ctx)
	task, err := s.getTask(ctx, req.ContainerID)
	if err != nil {
		return err
	}
	return setupCNINetwork(ctx, task, req.ContainerID, req.CNIBinDir, req.CNIConfDir)
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
