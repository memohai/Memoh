package mcp

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/containerd/errdefs"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/mcp/mcpclient"
)

type legacyRouteTestService struct {
	container ctr.ContainerInfo
	created   bool

	createCalls int
	startCalls  int
	deleteCalls int
	removeNet   int
	deleteTask  int

	getContainerBeforeCreateErr error
}

func (_ *legacyRouteTestService) PullImage(context.Context, string, *ctr.PullImageOptions) (ctr.ImageInfo, error) {
	return ctr.ImageInfo{}, nil
}

func (_ *legacyRouteTestService) GetImage(context.Context, string) (ctr.ImageInfo, error) {
	return ctr.ImageInfo{}, nil
}

func (_ *legacyRouteTestService) ListImages(context.Context) ([]ctr.ImageInfo, error) {
	return nil, nil
}

func (_ *legacyRouteTestService) DeleteImage(context.Context, string, *ctr.DeleteImageOptions) error {
	return nil
}

func (_ *legacyRouteTestService) ResolveRemoteDigest(context.Context, string) (string, error) {
	return "", nil
}

func (s *legacyRouteTestService) CreateContainer(_ context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	s.createCalls++
	s.created = true
	s.container = ctr.ContainerInfo{
		ID:          req.ID,
		Image:       req.ImageRef,
		Labels:      req.Labels,
		Snapshotter: req.Snapshotter,
		SnapshotKey: req.ID,
	}
	return s.container, nil
}

func (s *legacyRouteTestService) GetContainer(context.Context, string) (ctr.ContainerInfo, error) {
	if !s.created {
		if s.getContainerBeforeCreateErr != nil {
			return ctr.ContainerInfo{}, s.getContainerBeforeCreateErr
		}
		return ctr.ContainerInfo{}, errdefs.ErrNotFound
	}
	return s.container, nil
}

func (s *legacyRouteTestService) ListContainers(context.Context) ([]ctr.ContainerInfo, error) {
	if !s.created {
		return nil, nil
	}
	return []ctr.ContainerInfo{s.container}, nil
}

func (s *legacyRouteTestService) DeleteContainer(context.Context, string, *ctr.DeleteContainerOptions) error {
	s.deleteCalls++
	s.created = false
	return nil
}

func (_ *legacyRouteTestService) ListContainersByLabel(context.Context, string, string) ([]ctr.ContainerInfo, error) {
	return nil, nil
}

func (s *legacyRouteTestService) StartContainer(context.Context, string, *ctr.StartTaskOptions) error {
	s.startCalls++
	return nil
}

func (_ *legacyRouteTestService) StopContainer(context.Context, string, *ctr.StopTaskOptions) error {
	return nil
}

func (s *legacyRouteTestService) DeleteTask(context.Context, string, *ctr.DeleteTaskOptions) error {
	s.deleteTask++
	return nil
}

func (_ *legacyRouteTestService) GetTaskInfo(context.Context, string) (ctr.TaskInfo, error) {
	return ctr.TaskInfo{}, errdefs.ErrNotFound
}

func (_ *legacyRouteTestService) ListTasks(context.Context, *ctr.ListTasksOptions) ([]ctr.TaskInfo, error) {
	return nil, nil
}

func (_ *legacyRouteTestService) SetupNetwork(context.Context, ctr.NetworkSetupRequest) (ctr.NetworkResult, error) {
	return ctr.NetworkResult{IP: "10.0.0.2"}, nil
}

func (s *legacyRouteTestService) RemoveNetwork(context.Context, ctr.NetworkSetupRequest) error {
	s.removeNet++
	return nil
}

func (_ *legacyRouteTestService) CommitSnapshot(context.Context, string, string, string) error {
	return nil
}

func (_ *legacyRouteTestService) ListSnapshots(context.Context, string) ([]ctr.SnapshotInfo, error) {
	return nil, nil
}

func (_ *legacyRouteTestService) PrepareSnapshot(context.Context, string, string, string) error {
	return nil
}

func (_ *legacyRouteTestService) CreateContainerFromSnapshot(context.Context, ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	return ctr.ContainerInfo{}, nil
}

func (_ *legacyRouteTestService) SnapshotMounts(context.Context, string, string) ([]ctr.MountInfo, error) {
	return nil, ctr.ErrNotSupported
}

func newLegacyRouteTestManager(t *testing.T, svc ctr.Service, cfg config.MCPConfig) *Manager {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	m := &Manager{
		service:        svc,
		cfg:            cfg,
		namespace:      config.DefaultNamespace,
		containerLocks: make(map[string]*sync.Mutex),
		legacyIPs:      make(map[string]string),
		logger:         logger,
	}
	m.containerID = func(botID string) string { return ContainerPrefix + botID }
	m.grpcPool = mcpclient.NewPool(m.dialTarget)
	return m
}

func TestStartWithImageClearsLegacyRouteForBridgeContainer(t *testing.T) {
	dataRoot := t.TempDir()
	runtimeDir := filepath.Join(dataRoot, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}

	svc := &legacyRouteTestService{}
	m := newLegacyRouteTestManager(t, svc, config.MCPConfig{
		DataRoot:     dataRoot,
		RuntimeDir:   runtimeDir,
		Snapshotter:  "overlayfs",
		CNIBinaryDir: "/opt/cni/bin",
		CNIConfigDir: "/etc/cni/net.d",
	})

	botID := "00000000-0000-0000-0000-000000000001"
	m.SetLegacyIP(botID, "10.0.0.9")

	if got := m.dialTarget(botID); got != "10.0.0.9:9090" {
		t.Fatalf("expected legacy dial target before start, got %q", got)
	}

	if err := m.StartWithImage(context.Background(), botID, ""); err != nil {
		t.Fatalf("StartWithImage failed: %v", err)
	}

	if got := m.dialTarget(botID); got != "unix://"+filepath.Join(dataRoot, "run", botID, "mcp.sock") {
		t.Fatalf("expected unix dial target after bridge start, got %q", got)
	}
	if svc.createCalls != 1 || svc.startCalls != 1 {
		t.Fatalf("expected create/start once, got create=%d start=%d", svc.createCalls, svc.startCalls)
	}
}

func TestDeleteClearsLegacyRoute(t *testing.T) {
	svc := &legacyRouteTestService{created: true, container: ctr.ContainerInfo{ID: "mcp-00000000-0000-0000-0000-000000000001"}}
	m := newLegacyRouteTestManager(t, svc, config.MCPConfig{
		DataRoot:     t.TempDir(),
		Snapshotter:  "overlayfs",
		CNIBinaryDir: "/opt/cni/bin",
		CNIConfigDir: "/etc/cni/net.d",
	})

	botID := "00000000-0000-0000-0000-000000000001"
	m.SetLegacyIP(botID, "10.0.0.9")

	if err := m.Delete(context.Background(), botID, false); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if got := m.dialTarget(botID); got == "10.0.0.9:9090" {
		t.Fatalf("expected legacy TCP target to be cleared, got %q", got)
	}
	if svc.removeNet != 1 || svc.deleteTask != 1 || svc.deleteCalls != 1 {
		t.Fatalf("expected delete cleanup once, got removeNet=%d deleteTask=%d delete=%d", svc.removeNet, svc.deleteTask, svc.deleteCalls)
	}
}
