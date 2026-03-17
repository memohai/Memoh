package workspace

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/containerd/errdefs"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type legacyRouteTestService struct {
	container ctr.ContainerInfo
	created   bool
	byLabel   []ctr.ContainerInfo

	createCalls int
	startCalls  int
	deleteCalls int
	removeNet   int
	deleteTask  int
	setupNet    int

	getContainerBeforeCreateErr error
	setupNetworkResults         []ctr.NetworkResult
	setupNetworkErrs            []error
}

func (*legacyRouteTestService) PullImage(context.Context, string, *ctr.PullImageOptions) (ctr.ImageInfo, error) {
	return ctr.ImageInfo{}, nil
}

func (*legacyRouteTestService) GetImage(context.Context, string) (ctr.ImageInfo, error) {
	return ctr.ImageInfo{}, nil
}

func (*legacyRouteTestService) ListImages(context.Context) ([]ctr.ImageInfo, error) {
	return nil, nil
}

func (*legacyRouteTestService) DeleteImage(context.Context, string, *ctr.DeleteImageOptions) error {
	return nil
}

func (*legacyRouteTestService) ResolveRemoteDigest(context.Context, string) (string, error) {
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

func (s *legacyRouteTestService) ListContainersByLabel(context.Context, string, string) ([]ctr.ContainerInfo, error) {
	return s.byLabel, nil
}

func (s *legacyRouteTestService) StartContainer(context.Context, string, *ctr.StartTaskOptions) error {
	s.startCalls++
	return nil
}

func (*legacyRouteTestService) StopContainer(context.Context, string, *ctr.StopTaskOptions) error {
	return nil
}

func (s *legacyRouteTestService) DeleteTask(context.Context, string, *ctr.DeleteTaskOptions) error {
	s.deleteTask++
	return nil
}

func (*legacyRouteTestService) GetTaskInfo(context.Context, string) (ctr.TaskInfo, error) {
	return ctr.TaskInfo{}, errdefs.ErrNotFound
}

func (*legacyRouteTestService) ListTasks(context.Context, *ctr.ListTasksOptions) ([]ctr.TaskInfo, error) {
	return nil, nil
}

func (s *legacyRouteTestService) SetupNetwork(context.Context, ctr.NetworkSetupRequest) (ctr.NetworkResult, error) {
	idx := s.setupNet
	s.setupNet++
	if idx < len(s.setupNetworkErrs) && s.setupNetworkErrs[idx] != nil {
		return ctr.NetworkResult{}, s.setupNetworkErrs[idx]
	}
	if idx < len(s.setupNetworkResults) {
		return s.setupNetworkResults[idx], nil
	}
	return ctr.NetworkResult{IP: "10.0.0.2"}, nil
}

func (s *legacyRouteTestService) RemoveNetwork(context.Context, ctr.NetworkSetupRequest) error {
	s.removeNet++
	return nil
}

func (*legacyRouteTestService) CommitSnapshot(context.Context, string, string, string) error {
	return nil
}

func (*legacyRouteTestService) ListSnapshots(context.Context, string) ([]ctr.SnapshotInfo, error) {
	return nil, nil
}

func (*legacyRouteTestService) PrepareSnapshot(context.Context, string, string, string) error {
	return nil
}

func (*legacyRouteTestService) CreateContainerFromSnapshot(context.Context, ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	return ctr.ContainerInfo{}, nil
}

func (*legacyRouteTestService) SnapshotMounts(context.Context, string, string) ([]ctr.MountInfo, error) {
	return nil, ctr.ErrNotSupported
}

func newLegacyRouteTestManager(t *testing.T, svc ctr.Service, cfg config.WorkspaceConfig) *Manager {
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
	m.grpcPool = bridge.NewPool(m.dialTarget)
	return m
}

func TestStartWithImageClearsLegacyRouteForBridgeContainer(t *testing.T) {
	dataRoot := t.TempDir()
	runtimeDir := filepath.Join(dataRoot, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o750); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}

	svc := &legacyRouteTestService{}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{
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

	if got := m.dialTarget(botID); got != "unix://"+filepath.Join(dataRoot, "run", botID, "bridge.sock") {
		t.Fatalf("expected unix dial target after bridge start, got %q", got)
	}
	if svc.createCalls != 1 || svc.startCalls != 1 {
		t.Fatalf("expected create/start once, got create=%d start=%d", svc.createCalls, svc.startCalls)
	}
}

func TestDeleteClearsLegacyRoute(t *testing.T) {
	svc := &legacyRouteTestService{created: true, container: ctr.ContainerInfo{ID: "workspace-00000000-0000-0000-0000-000000000001"}}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{
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

func TestSetupNetworkAndGetIPRejectsEmptyIP(t *testing.T) {
	svc := &legacyRouteTestService{
		setupNetworkResults: []ctr.NetworkResult{{IP: ""}, {IP: "10.0.0.3"}},
	}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{
		CNIBinaryDir: "/opt/cni/bin",
		CNIConfigDir: "/etc/cni/net.d",
	})

	ip, err := m.setupNetworkAndGetIP(context.Background(), "workspace-bot")
	if err != nil {
		t.Fatalf("setupNetworkAndGetIP failed: %v", err)
	}
	if ip != "10.0.0.3" {
		t.Fatalf("expected retry IP, got %q", ip)
	}
	if svc.setupNet != 2 {
		t.Fatalf("expected two network setup attempts, got %d", svc.setupNet)
	}
}

func TestContainerIDPrefersCurrentLabelSearch(t *testing.T) {
	t.Parallel()

	botID := "00000000-0000-0000-0000-000000000001"
	svc := &legacyRouteTestService{
		byLabel: []ctr.ContainerInfo{{
			ID:        "workspace-from-label",
			Labels:    map[string]string{BotLabelKey: botID},
			UpdatedAt: time.Now(),
		}},
	}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{})

	containerID, err := m.ContainerID(context.Background(), botID)
	if err != nil {
		t.Fatalf("ContainerID failed: %v", err)
	}
	if containerID != "workspace-from-label" {
		t.Fatalf("expected label-resolved container ID, got %q", containerID)
	}
}

func TestContainerIDFallsBackToNameInference(t *testing.T) {
	t.Parallel()

	botID := "00000000-0000-0000-0000-000000000001"
	svc := &legacyRouteTestService{
		created: true,
		container: ctr.ContainerInfo{
			ID:        ContainerPrefix + botID,
			UpdatedAt: time.Now(),
		},
	}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{})

	containerID, err := m.ContainerID(context.Background(), botID)
	if err != nil {
		t.Fatalf("ContainerID failed: %v", err)
	}
	if containerID != ContainerPrefix+botID {
		t.Fatalf("expected inferred container ID, got %q", containerID)
	}
}
