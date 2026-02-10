package containerd

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	tasksv1 "github.com/containerd/containerd/api/services/tasks/v1"
	tasktypes "github.com/containerd/containerd/api/types/task"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/memohai/memoh/internal/config"
	"github.com/opencontainers/go-digest"
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
	SpecOpts    []oci.SpecOpts
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

type SnapshotCommitResult struct {
	VersionSnapshotID string
	ActiveSnapshotID  string
}

type ListTasksOptions struct {
	Filter string
}

type TaskInfo struct {
	ContainerID string
	ID          string
	PID         uint32
	Status      tasktypes.Status
	ExitStatus  uint32
}

type Service interface {
	PullImage(ctx context.Context, ref string, opts *PullImageOptions) (containerd.Image, error)
	GetImage(ctx context.Context, ref string) (containerd.Image, error)
	ListImages(ctx context.Context) ([]containerd.Image, error)
	DeleteImage(ctx context.Context, ref string, opts *DeleteImageOptions) error

	CreateContainer(ctx context.Context, req CreateContainerRequest) (containerd.Container, error)
	GetContainer(ctx context.Context, id string) (containerd.Container, error)
	ListContainers(ctx context.Context) ([]containerd.Container, error)
	DeleteContainer(ctx context.Context, id string, opts *DeleteContainerOptions) error

	StartTask(ctx context.Context, containerID string, opts *StartTaskOptions) (containerd.Task, error)
	GetTask(ctx context.Context, containerID string) (containerd.Task, error)
	ListTasks(ctx context.Context, opts *ListTasksOptions) ([]TaskInfo, error)
	StopTask(ctx context.Context, containerID string, opts *StopTaskOptions) error
	DeleteTask(ctx context.Context, containerID string, opts *DeleteTaskOptions) error
	ExecTask(ctx context.Context, containerID string, req ExecTaskRequest) (ExecTaskResult, error)
	ExecTaskStreaming(ctx context.Context, containerID string, req ExecTaskRequest) (*ExecTaskSession, error)
	ListContainersByLabel(ctx context.Context, key, value string) ([]containerd.Container, error)
	CommitSnapshot(ctx context.Context, snapshotter, name, key string) error
	ListSnapshots(ctx context.Context, snapshotter string) ([]snapshots.Info, error)
	PrepareSnapshot(ctx context.Context, snapshotter, key, parent string) error
	CreateContainerFromSnapshot(ctx context.Context, req CreateContainerRequest) (containerd.Container, error)
	SnapshotMounts(ctx context.Context, snapshotter, key string) ([]mount.Mount, error)
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

func (s *DefaultService) PullImage(ctx context.Context, ref string, opts *PullImageOptions) (containerd.Image, error) {
	if ref == "" {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	pullOpts := []containerd.RemoteOpt{}
	if opts == nil || opts.Unpack {
		pullOpts = append(pullOpts, containerd.WithPullUnpack)
	}
	if opts != nil && opts.Snapshotter != "" {
		pullOpts = append(pullOpts, containerd.WithPullSnapshotter(opts.Snapshotter))
	}

	return s.client.Pull(ctx, ref, pullOpts...)
}

func (s *DefaultService) GetImage(ctx context.Context, ref string) (containerd.Image, error) {
	if ref == "" {
		return nil, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	return s.client.GetImage(ctx, ref)
}

func (s *DefaultService) ListImages(ctx context.Context) ([]containerd.Image, error) {
	ctx = s.withNamespace(ctx)
	return s.client.ListImages(ctx)
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

func (s *DefaultService) CreateContainer(ctx context.Context, req CreateContainerRequest) (containerd.Container, error) {
	if req.ID == "" || req.ImageRef == "" {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	ctx, done, err := s.client.WithLease(ctx)
	if err != nil {
		return nil, err
	}
	defer done(ctx)
	image, err := s.getImageWithFallback(ctx, req.ImageRef)
	if err != nil {
		pullOpts := &PullImageOptions{
			Unpack:      true,
			Snapshotter: req.Snapshotter,
		}
		image, err = s.PullImage(ctx, req.ImageRef, pullOpts)
		if err != nil {
			return nil, err
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
	if len(req.SpecOpts) > 0 {
		specOpts = append(specOpts, req.SpecOpts...)
	}

	containerOpts := []containerd.NewContainerOpts{
		containerd.WithImage(image),
	}
	if req.Snapshotter != "" {
		containerOpts = append(containerOpts, containerd.WithSnapshotter(req.Snapshotter))
	}
	if req.Snapshotter != "" {
		parent, err := s.snapshotParentFromLayers(ctx, image)
		if err != nil {
			return nil, err
		}
		ok, err := s.snapshotExists(ctx, req.Snapshotter, parent)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("parent snapshot %s does not exist", parent)
		}
		if err := s.prepareSnapshot(ctx, req.Snapshotter, snapshotID, parent); err != nil {
			return nil, err
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

	return s.client.NewContainer(ctx, req.ID, containerOpts...)
}

func (s *DefaultService) snapshotParentFromLayers(ctx context.Context, image containerd.Image) (string, error) {
	manifest, err := images.Manifest(ctx, s.client.ContentStore(), image.Target(), platforms.Default())
	if err != nil {
		return "", err
	}
	if len(manifest.Layers) == 0 {
		return "", fmt.Errorf("image has no layer descriptors")
	}
	diffIDs := make([]digest.Digest, 0, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		blob, err := content.ReadBlob(ctx, s.client.ContentStore(), layer)
		if err != nil {
			return "", err
		}
		reader := bytes.NewReader(blob)
		var r io.ReadCloser
		if strings.Contains(layer.MediaType, "gzip") {
			r, err = gzip.NewReader(reader)
			if err != nil {
				return "", err
			}
		} else {
			r = io.NopCloser(reader)
		}

		digester := digest.Canonical.Digester()
		if _, err := io.Copy(digester.Hash(), r); err != nil {
			_ = r.Close()
			return "", err
		}
		_ = r.Close()
		diffIDs = append(diffIDs, digester.Digest())
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
	image, err := s.GetImage(ctx, ref)
	if err == nil {
		return image, nil
	}
	if strings.HasPrefix(ref, "docker.io/library/") {
		alt := strings.TrimPrefix(ref, "docker.io/library/")
		image, altErr := s.GetImage(ctx, alt)
		if altErr == nil {
			return image, nil
		}
	}
	images, listErr := s.ListImages(ctx)
	if listErr == nil {
		for _, img := range images {
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

func (s *DefaultService) GetContainer(ctx context.Context, id string) (containerd.Container, error) {
	if id == "" {
		return nil, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	return s.client.LoadContainer(ctx, id)
}

func (s *DefaultService) ListContainers(ctx context.Context) ([]containerd.Container, error) {
	ctx = s.withNamespace(ctx)
	return s.client.Containers(ctx)
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

func (s *DefaultService) StartTask(ctx context.Context, containerID string, opts *StartTaskOptions) (containerd.Task, error) {
	if containerID == "" {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	container, err := s.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if err := task.Start(ctx); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *DefaultService) GetTask(ctx context.Context, containerID string) (containerd.Task, error) {
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
			Status:      task.Status,
			ExitStatus:  task.ExitStatus,
		})
	}

	return tasks, nil
}

func (s *DefaultService) StopTask(ctx context.Context, containerID string, opts *StopTaskOptions) error {
	if containerID == "" {
		return ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	task, err := s.GetTask(ctx, containerID)
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
	task, err := s.GetTask(ctx, containerID)
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

	task, err := container.Task(ctx, nil)
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

	task, err := container.Task(ctx, nil)
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
	fifoDir := strings.TrimSpace(req.FIFODir)
	if fifoDir == "" {
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			fifoDir = filepath.Join(homeDir, ".memoh", "containerd-fifo")
		} else {
			fifoDir = "/tmp/memoh-containerd-fifo"
		}
	}
	if err := os.MkdirAll(fifoDir, 0o755); err != nil {
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

func (s *DefaultService) ListContainersByLabel(ctx context.Context, key, value string) ([]containerd.Container, error) {
	if key == "" {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)
	containers, err := s.client.Containers(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]containerd.Container, 0, len(containers))
	for _, container := range containers {
		info, err := container.Info(ctx)
		if err != nil {
			return nil, err
		}
		if labelValue, ok := info.Labels[key]; ok && (value == "" || value == labelValue) {
			filtered = append(filtered, container)
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

func (s *DefaultService) ListSnapshots(ctx context.Context, snapshotter string) ([]snapshots.Info, error) {
	if snapshotter == "" {
		return nil, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	infos := []snapshots.Info{}
	if err := s.client.SnapshotService(snapshotter).Walk(ctx, func(ctx context.Context, info snapshots.Info) error {
		infos = append(infos, info)
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

func (s *DefaultService) CreateContainerFromSnapshot(ctx context.Context, req CreateContainerRequest) (containerd.Container, error) {
	if req.ID == "" || req.SnapshotID == "" {
		return nil, ErrInvalidArgument
	}

	ctx = s.withNamespace(ctx)

	imageRef := req.ImageRef
	if imageRef == "" {
		return nil, ErrInvalidArgument
	}

	image, err := s.GetImage(ctx, imageRef)
	if err != nil {
		image, err = s.PullImage(ctx, imageRef, &PullImageOptions{
			Unpack:      true,
			Snapshotter: req.Snapshotter,
		})
		if err != nil {
			return nil, err
		}
	}

	specOpts := []oci.SpecOpts{
		oci.WithDefaultSpecForPlatform("linux/" + runtime.GOARCH),
		oci.WithImageConfig(image),
	}
	if len(req.SpecOpts) > 0 {
		specOpts = append(specOpts, req.SpecOpts...)
	}

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

	return s.client.NewContainer(ctx, req.ID, containerOpts...)
}

func (s *DefaultService) SnapshotMounts(ctx context.Context, snapshotter, key string) ([]mount.Mount, error) {
	if snapshotter == "" || key == "" {
		return nil, ErrInvalidArgument
	}
	ctx = s.withNamespace(ctx)
	return s.client.SnapshotService(snapshotter).Mounts(ctx, key)
}

func (s *DefaultService) withNamespace(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, s.namespace)
}
