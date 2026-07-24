package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type runtimeRouterRequiredSpy struct {
	runtimeService

	calls []string
	args  map[string]any
	errs  map[string]error

	containers      []ctr.ContainerInfo
	labelContainers []ctr.ContainerInfo
	tasks           []ctr.TaskInfo
	snapshots       []ctr.SnapshotInfo
	networkResult   ctr.NetworkResult
}

var _ runtimeService = (*runtimeRouterRequiredSpy)(nil)

func newRuntimeRouterRequiredSpy() *runtimeRouterRequiredSpy {
	return &runtimeRouterRequiredSpy{
		runtimeService: &legacyRouteTestService{},
		args:           make(map[string]any),
		errs:           make(map[string]error),
		networkResult:  ctr.NetworkResult{IP: "10.0.0.8"},
	}
}

func (s *runtimeRouterRequiredSpy) record(name string, arg any) error {
	s.calls = append(s.calls, name)
	s.args[name] = arg
	return s.errs[name]
}

func (s *runtimeRouterRequiredSpy) CreateContainer(_ context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	err := s.record("CreateContainer", req)
	return ctr.ContainerInfo{
		ID:         req.ID,
		Image:      req.ImageRef,
		StorageRef: req.StorageRef,
		Runtime:    ctr.RuntimeInfo{Name: "container"},
	}, err
}

func (s *runtimeRouterRequiredSpy) GetContainer(_ context.Context, id string) (ctr.ContainerInfo, error) {
	err := s.record("GetContainer", id)
	return ctr.ContainerInfo{ID: id, Runtime: ctr.RuntimeInfo{Name: "container"}}, err
}

func (s *runtimeRouterRequiredSpy) ListContainers(context.Context) ([]ctr.ContainerInfo, error) {
	err := s.record("ListContainers", nil)
	return s.containers, err
}

func (s *runtimeRouterRequiredSpy) DeleteContainer(_ context.Context, id string, opts *ctr.DeleteContainerOptions) error {
	return s.record("DeleteContainer", struct {
		id   string
		opts *ctr.DeleteContainerOptions
	}{id: id, opts: opts})
}

func (s *runtimeRouterRequiredSpy) ListContainersByLabel(_ context.Context, key, value string) ([]ctr.ContainerInfo, error) {
	err := s.record("ListContainersByLabel", [2]string{key, value})
	return s.labelContainers, err
}

func (s *runtimeRouterRequiredSpy) StartContainer(_ context.Context, id string, opts *ctr.StartTaskOptions) error {
	return s.record("StartContainer", struct {
		id   string
		opts *ctr.StartTaskOptions
	}{id: id, opts: opts})
}

func (s *runtimeRouterRequiredSpy) StopContainer(_ context.Context, id string, opts *ctr.StopTaskOptions) error {
	return s.record("StopContainer", struct {
		id   string
		opts *ctr.StopTaskOptions
	}{id: id, opts: opts})
}

func (s *runtimeRouterRequiredSpy) DeleteTask(_ context.Context, id string, opts *ctr.DeleteTaskOptions) error {
	return s.record("DeleteTask", struct {
		id   string
		opts *ctr.DeleteTaskOptions
	}{id: id, opts: opts})
}

func (s *runtimeRouterRequiredSpy) GetTaskInfo(_ context.Context, id string) (ctr.TaskInfo, error) {
	err := s.record("GetTaskInfo", id)
	return ctr.TaskInfo{ContainerID: id, ID: "task-" + id, Status: ctr.TaskStatusRunning}, err
}

func (s *runtimeRouterRequiredSpy) GetContainerMetrics(_ context.Context, id string) (ctr.ContainerMetrics, error) {
	err := s.record("GetContainerMetrics", id)
	return ctr.ContainerMetrics{CPU: &ctr.CPUMetrics{UsagePercent: 12.5}}, err
}

func (s *runtimeRouterRequiredSpy) ListTasks(_ context.Context, opts *ctr.ListTasksOptions) ([]ctr.TaskInfo, error) {
	err := s.record("ListTasks", opts)
	return s.tasks, err
}

func (s *runtimeRouterRequiredSpy) SetupNetwork(_ context.Context, req ctr.NetworkRequest) (ctr.NetworkResult, error) {
	err := s.record("SetupNetwork", req)
	return s.networkResult, err
}

func (s *runtimeRouterRequiredSpy) RemoveNetwork(_ context.Context, req ctr.NetworkRequest) error {
	return s.record("RemoveNetwork", req)
}

func (s *runtimeRouterRequiredSpy) CheckNetwork(_ context.Context, req ctr.NetworkRequest) error {
	return s.record("CheckNetwork", req)
}

func (s *runtimeRouterRequiredSpy) CommitSnapshot(_ context.Context, req ctr.CommitSnapshotRequest) error {
	return s.record("CommitSnapshot", req)
}

func (s *runtimeRouterRequiredSpy) ListSnapshots(_ context.Context, req ctr.ListSnapshotsRequest) ([]ctr.SnapshotInfo, error) {
	err := s.record("ListSnapshots", req)
	return s.snapshots, err
}

func (s *runtimeRouterRequiredSpy) PrepareSnapshot(_ context.Context, req ctr.PrepareSnapshotRequest) error {
	return s.record("PrepareSnapshot", req)
}

func (s *runtimeRouterRequiredSpy) RestoreContainer(_ context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	err := s.record("RestoreContainer", req)
	return ctr.ContainerInfo{ID: req.ID, StorageRef: req.StorageRef}, err
}

type runtimeRouterCapabilitySpy struct {
	*runtimeRouterRequiredSpy
	capabilityErrs map[string]error
}

var _ ctr.ImageService = (*runtimeRouterCapabilitySpy)(nil)

func newRuntimeRouterCapabilitySpy() *runtimeRouterCapabilitySpy {
	return &runtimeRouterCapabilitySpy{
		runtimeRouterRequiredSpy: newRuntimeRouterRequiredSpy(),
		capabilityErrs:           make(map[string]error),
	}
}

func (*runtimeRouterCapabilitySpy) RuntimeType() string {
	return "io.containerd.test.v1"
}

func (s *runtimeRouterCapabilitySpy) PullImage(_ context.Context, ref string, opts *ctr.PullImageOptions) (ctr.ImageInfo, error) {
	if err := s.record("PullImage", struct {
		ref  string
		opts *ctr.PullImageOptions
	}{ref: ref, opts: opts}); err != nil {
		return ctr.ImageInfo{}, err
	}
	return ctr.ImageInfo{Name: ref, ID: "pulled"}, s.capabilityErrs["PullImage"]
}

func (s *runtimeRouterCapabilitySpy) GetImage(_ context.Context, ref string) (ctr.ImageInfo, error) {
	if err := s.record("GetImage", ref); err != nil {
		return ctr.ImageInfo{}, err
	}
	return ctr.ImageInfo{Name: ref, ID: "present"}, s.capabilityErrs["GetImage"]
}

func (s *runtimeRouterCapabilitySpy) ListImages(context.Context) ([]ctr.ImageInfo, error) {
	if err := s.record("ListImages", nil); err != nil {
		return nil, err
	}
	return []ctr.ImageInfo{{Name: "image-a"}}, s.capabilityErrs["ListImages"]
}

func (s *runtimeRouterCapabilitySpy) DeleteImage(_ context.Context, ref string, opts *ctr.DeleteImageOptions) error {
	if err := s.record("DeleteImage", struct {
		ref  string
		opts *ctr.DeleteImageOptions
	}{ref: ref, opts: opts}); err != nil {
		return err
	}
	return s.capabilityErrs["DeleteImage"]
}

func (s *runtimeRouterCapabilitySpy) ResolveRemoteDigest(_ context.Context, ref string) (string, error) {
	if err := s.record("ResolveRemoteDigest", ref); err != nil {
		return "", err
	}
	return "sha256:test", s.capabilityErrs["ResolveRemoteDigest"]
}

func (s *runtimeRouterCapabilitySpy) SnapshotMounts(_ context.Context, snapshotter, key string) ([]ctr.MountInfo, error) {
	if err := s.record("SnapshotMounts", [2]string{snapshotter, key}); err != nil {
		return nil, err
	}
	return []ctr.MountInfo{{Type: "bind", Source: "/source", Target: "/target"}}, s.capabilityErrs["SnapshotMounts"]
}

func (*runtimeRouterCapabilitySpy) BridgeTarget(botID string) string {
	return "unix:///run/" + botID + ".sock"
}

func TestRuntimeRouterRouteSelection(t *testing.T) {
	containerSvc := newRuntimeRouterRequiredSpy()
	enabledLocal := newRuntimeRouterTestLocal(t, true)
	disabledLocal := newRuntimeRouterTestLocal(t, false)
	enabledRouter := NewRuntimeRouter(containerSvc, enabledLocal)
	disabledRouter := NewRuntimeRouter(containerSvc, disabledLocal)

	if !enabledRouter.LocalEnabled() {
		t.Fatal("enabled local runtime reported disabled")
	}
	if disabledRouter.LocalEnabled() {
		t.Fatal("disabled local runtime reported enabled")
	}

	tests := []struct {
		name        string
		router      *RuntimeRouter
		id          string
		driver      string
		localByID   bool
		localCreate bool
	}{
		{name: "local prefix", router: enabledRouter, id: "local-bot-a", localByID: true, localCreate: true},
		{name: "trimmed local prefix", router: enabledRouter, id: "  local-bot-a  ", localByID: true, localCreate: true},
		{name: "ordinary id", router: enabledRouter, id: "memoh-bot-a"},
		{name: "prefix is case sensitive", router: enabledRouter, id: "LOCAL-bot-a"},
		{name: "prefix wins over driver", router: enabledRouter, id: "local-bot-a", driver: "overlayfs", localByID: true, localCreate: true},
		{name: "driver affects create only", router: enabledRouter, id: "workspace-bot-a", driver: " local ", localCreate: true},
		{name: "driver is case sensitive", router: enabledRouter, id: "workspace-bot-a", driver: "LOCAL"},
		{name: "disabled local ignores hints", router: disabledRouter, id: "local-bot-a", driver: "local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantByID := runtimeService(containerSvc)
			if tt.localByID {
				wantByID = tt.router.local
			}
			if got := tt.router.routeByID(tt.id); got != wantByID {
				t.Fatalf("routeByID(%q) = %T, want %T", tt.id, got, wantByID)
			}
			wantCreate := runtimeService(containerSvc)
			if tt.localCreate {
				wantCreate = tt.router.local
			}
			req := ctr.CreateContainerRequest{
				ID:         tt.id,
				StorageRef: ctr.StorageRef{Driver: tt.driver},
			}
			if got := tt.router.routeCreate(req); got != wantCreate {
				t.Fatalf("routeCreate(%q, driver=%q) = %T, want %T", tt.id, tt.driver, got, wantCreate)
			}
		})
	}
}

func TestRuntimeRouterDelegatesLifecycleWithoutRewritingArguments(t *testing.T) {
	ctx := context.Background()
	containerSvc := newRuntimeRouterRequiredSpy()
	local := newRuntimeRouterTestLocal(t, true)
	router := NewRuntimeRouter(containerSvc, local)

	localReq := ctr.CreateContainerRequest{
		ID:         "local-bot-local",
		ImageRef:   "local",
		StorageRef: ctr.StorageRef{Driver: localRuntimeName, Key: filepath.Join(t.TempDir(), "workspace")},
	}
	localInfo, err := router.CreateContainer(ctx, localReq)
	if err != nil {
		t.Fatalf("create local container: %v", err)
	}
	if localInfo.Runtime.Name != localRuntimeName {
		t.Fatalf("local runtime = %q, want %q", localInfo.Runtime.Name, localRuntimeName)
	}
	if _, err := router.GetContainer(ctx, localReq.ID); err != nil {
		t.Fatalf("get local container: %v", err)
	}
	if err := router.StartContainer(ctx, localReq.ID, &ctr.StartTaskOptions{Terminal: true}); err != nil {
		t.Fatalf("start local container: %v", err)
	}
	task, err := router.GetTaskInfo(ctx, localReq.ID)
	if err != nil {
		t.Fatalf("get local task: %v", err)
	}
	if task.Status != ctr.TaskStatusRunning {
		t.Fatalf("local task status = %s, want running", task.Status)
	}
	if _, err := router.GetContainerMetrics(ctx, localReq.ID); !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("local metrics error = %v, want ErrNotSupported", err)
	}
	if err := router.StopContainer(ctx, localReq.ID, &ctr.StopTaskOptions{Force: true}); err != nil {
		t.Fatalf("stop local container: %v", err)
	}
	if err := router.DeleteTask(ctx, localReq.ID, &ctr.DeleteTaskOptions{Force: true}); err != nil {
		t.Fatalf("delete local task: %v", err)
	}
	localDeleteOpts := &ctr.DeleteContainerOptions{}
	if err := router.DeleteContainer(ctx, localReq.ID, localDeleteOpts); err != nil {
		t.Fatalf("delete local container: %v", err)
	}
	if len(containerSvc.calls) != 0 {
		t.Fatalf("local lifecycle reached container backend: %v", containerSvc.calls)
	}

	driverOnlyReq := ctr.CreateContainerRequest{
		ID:         "workspace-driver-only",
		ImageRef:   "local",
		StorageRef: ctr.StorageRef{Driver: " local ", Key: filepath.Join(t.TempDir(), "driver-workspace")},
		Labels:     map[string]string{BotLabelKey: "bot-driver-only"},
	}
	if _, err := router.CreateContainer(ctx, driverOnlyReq); err != nil {
		t.Fatalf("create driver-only local container: %v", err)
	}
	if _, err := local.GetContainer(ctx, driverOnlyReq.ID); err != nil {
		t.Fatalf("driver-only container was not created locally: %v", err)
	}
	got, err := router.GetContainer(ctx, driverOnlyReq.ID)
	if err != nil {
		t.Fatalf("get driver-only container through router: %v", err)
	}
	if got.Runtime.Name != "container" {
		t.Fatalf("driver-only follow-up runtime = %q, want container", got.Runtime.Name)
	}

	containerSvc.calls = nil
	containerReq := ctr.CreateContainerRequest{
		ID:         "memoh-bot-container",
		ImageRef:   "image:test",
		StorageRef: ctr.StorageRef{Driver: "overlayfs", Key: "active"},
	}
	if _, err := router.CreateContainer(ctx, containerReq); err != nil {
		t.Fatalf("create container backend: %v", err)
	}
	if !reflect.DeepEqual(containerSvc.args["CreateContainer"], containerReq) {
		t.Fatalf("create request changed: got %#v want %#v", containerSvc.args["CreateContainer"], containerReq)
	}
	if _, err := router.GetContainer(ctx, containerReq.ID); err != nil {
		t.Fatalf("get container backend: %v", err)
	}
	startOpts := &ctr.StartTaskOptions{Terminal: true}
	if err := router.StartContainer(ctx, containerReq.ID, startOpts); err != nil {
		t.Fatalf("start container backend: %v", err)
	}
	stopErr := errors.New("stop marker")
	containerSvc.errs["StopContainer"] = stopErr
	stopOpts := &ctr.StopTaskOptions{Force: true}
	if err := router.StopContainer(ctx, containerReq.ID, stopOpts); !errors.Is(err, stopErr) {
		t.Fatalf("stop error = %v, want marker", err)
	}
	deleteTaskOpts := &ctr.DeleteTaskOptions{Force: true}
	if err := router.DeleteTask(ctx, containerReq.ID, deleteTaskOpts); err != nil {
		t.Fatalf("delete task backend: %v", err)
	}
	if _, err := router.GetTaskInfo(ctx, containerReq.ID); err != nil {
		t.Fatalf("get task backend: %v", err)
	}
	metrics, err := router.GetContainerMetrics(ctx, containerReq.ID)
	if err != nil {
		t.Fatalf("get metrics backend: %v", err)
	}
	if metrics.CPU == nil || metrics.CPU.UsagePercent != 12.5 {
		t.Fatalf("metrics result changed: %#v", metrics)
	}
	deleteOpts := &ctr.DeleteContainerOptions{CleanupSnapshot: true}
	if err := router.DeleteContainer(ctx, containerReq.ID, deleteOpts); err != nil {
		t.Fatalf("delete container backend: %v", err)
	}

	wantCalls := []string{
		"CreateContainer",
		"GetContainer",
		"StartContainer",
		"StopContainer",
		"DeleteTask",
		"GetTaskInfo",
		"GetContainerMetrics",
		"DeleteContainer",
	}
	if !reflect.DeepEqual(containerSvc.calls, wantCalls) {
		t.Fatalf("container calls = %v, want %v", containerSvc.calls, wantCalls)
	}
	if got := containerSvc.args["GetContainer"]; got != containerReq.ID {
		t.Fatalf("get container ID = %#v, want %q", got, containerReq.ID)
	}
	if got := containerSvc.args["StartContainer"].(struct {
		id   string
		opts *ctr.StartTaskOptions
	}); got.id != containerReq.ID || got.opts != startOpts {
		t.Fatalf("start arguments changed: %#v", got)
	}
	if got := containerSvc.args["StopContainer"].(struct {
		id   string
		opts *ctr.StopTaskOptions
	}); got.id != containerReq.ID || got.opts != stopOpts {
		t.Fatalf("stop arguments changed: %#v", got)
	}
	if got := containerSvc.args["DeleteTask"].(struct {
		id   string
		opts *ctr.DeleteTaskOptions
	}); got.id != containerReq.ID || got.opts != deleteTaskOpts {
		t.Fatalf("delete task arguments changed: %#v", got)
	}
	if got := containerSvc.args["DeleteContainer"].(struct {
		id   string
		opts *ctr.DeleteContainerOptions
	}); got.id != containerReq.ID || got.opts != deleteOpts {
		t.Fatalf("delete arguments changed: %#v", got)
	}
	if got := containerSvc.args["GetTaskInfo"]; got != containerReq.ID {
		t.Fatalf("get task ID = %#v, want %q", got, containerReq.ID)
	}
	if got := containerSvc.args["GetContainerMetrics"]; got != containerReq.ID {
		t.Fatalf("get metrics ID = %#v, want %q", got, containerReq.ID)
	}
}

func TestRuntimeRouterCombinesListsAndDropsPartialResultsOnError(t *testing.T) {
	ctx := context.Background()
	containerSvc := newRuntimeRouterRequiredSpy()
	containerSvc.containers = []ctr.ContainerInfo{{ID: "memoh-container"}}
	containerSvc.labelContainers = []ctr.ContainerInfo{{ID: "memoh-container"}}
	containerSvc.tasks = []ctr.TaskInfo{{ContainerID: "memoh-container", ID: "task-container"}}

	local := newRuntimeRouterTestLocal(t, true)
	createRuntimeRouterLocalContainer(t, local, "local-bot-list", map[string]string{"group": "test"})
	if err := local.StartContainer(ctx, "local-bot-list", nil); err != nil {
		t.Fatalf("start local list fixture: %v", err)
	}
	router := NewRuntimeRouter(containerSvc, local)

	containers, err := router.ListContainers(ctx)
	if err != nil {
		t.Fatalf("list containers: %v", err)
	}
	if got := containerIDs(containers); !reflect.DeepEqual(got, []string{"memoh-container", "local-bot-list"}) {
		t.Fatalf("container order = %v", got)
	}

	byLabel, err := router.ListContainersByLabel(ctx, "group", "test")
	if err != nil {
		t.Fatalf("list containers by label: %v", err)
	}
	if got := containerIDs(byLabel); !reflect.DeepEqual(got, []string{"memoh-container", "local-bot-list"}) {
		t.Fatalf("label container order = %v", got)
	}
	if got := containerSvc.args["ListContainersByLabel"]; got != [2]string{"group", "test"} {
		t.Fatalf("label arguments = %#v", got)
	}

	taskOpts := &ctr.ListTasksOptions{}
	tasks, err := router.ListTasks(ctx, taskOpts)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if got := taskContainerIDs(tasks); !reflect.DeepEqual(got, []string{"memoh-container", "local-bot-list"}) {
		t.Fatalf("task order = %v", got)
	}
	if got := containerSvc.args["ListTasks"]; got != taskOpts {
		t.Fatalf("task options changed: got %p want %p", got, taskOpts)
	}

	operations := []struct {
		name   string
		invoke func(*RuntimeRouter) (bool, error)
	}{
		{
			name: "ListContainers",
			invoke: func(router *RuntimeRouter) (bool, error) {
				got, err := router.ListContainers(ctx)
				return got == nil, err
			},
		},
		{
			name: "ListContainersByLabel",
			invoke: func(router *RuntimeRouter) (bool, error) {
				got, err := router.ListContainersByLabel(ctx, "group", "test")
				return got == nil, err
			},
		},
		{
			name: "ListTasks",
			invoke: func(router *RuntimeRouter) (bool, error) {
				got, err := router.ListTasks(ctx, taskOpts)
				return got == nil, err
			},
		},
	}

	for _, op := range operations {
		t.Run(op.name+"/container failure", func(t *testing.T) {
			sentinel := errors.New("container list marker")
			spy := newRuntimeRouterRequiredSpy()
			spy.containers = containerSvc.containers
			spy.labelContainers = containerSvc.labelContainers
			spy.tasks = containerSvc.tasks
			spy.errs[op.name] = sentinel
			nilResult, err := op.invoke(NewRuntimeRouter(spy, newBrokenRuntimeRouterTestLocal(t)))
			if !nilResult || !errors.Is(err, sentinel) {
				t.Fatalf("result nil=%v err=%v, want nil and marker", nilResult, err)
			}
		})

		t.Run(op.name+"/local failure", func(t *testing.T) {
			spy := newRuntimeRouterRequiredSpy()
			spy.containers = containerSvc.containers
			spy.labelContainers = containerSvc.labelContainers
			spy.tasks = containerSvc.tasks
			nilResult, err := op.invoke(NewRuntimeRouter(spy, newBrokenRuntimeRouterTestLocal(t)))
			if !nilResult || err == nil {
				t.Fatalf("result nil=%v err=%v, want nil and local error", nilResult, err)
			}
		})
	}
}

func TestRuntimeRouterRoutesNetworkByContainerID(t *testing.T) {
	ctx := context.Background()
	containerSvc := newRuntimeRouterRequiredSpy()
	local := newRuntimeRouterTestLocal(t, true)
	router := NewRuntimeRouter(containerSvc, local)

	localReq := ctr.NetworkRequest{
		ContainerID: "local-bot-network",
		JoinTarget:  ctr.NetworkJoinTarget{Kind: "pid", PID: 42},
	}
	result, err := router.SetupNetwork(ctx, localReq)
	if err != nil {
		t.Fatalf("setup local network: %v", err)
	}
	if result.IP != "127.0.0.1" {
		t.Fatalf("local network IP = %q", result.IP)
	}
	if err := router.RemoveNetwork(ctx, localReq); err != nil {
		t.Fatalf("remove local network: %v", err)
	}
	if err := router.CheckNetwork(ctx, localReq); err != nil {
		t.Fatalf("check local network: %v", err)
	}
	if len(containerSvc.calls) != 0 {
		t.Fatalf("local network reached container backend: %v", containerSvc.calls)
	}

	containerReq := ctr.NetworkRequest{
		ContainerID: "memoh-bot-network",
		JoinTarget:  ctr.NetworkJoinTarget{Kind: "network-mode", Value: "container:peer"},
	}
	setupErr := errors.New("setup marker")
	containerSvc.errs["SetupNetwork"] = setupErr
	result, err = router.SetupNetwork(ctx, containerReq)
	if !errors.Is(err, setupErr) {
		t.Fatalf("setup error = %v, want marker", err)
	}
	if result != containerSvc.networkResult {
		t.Fatalf("setup result = %#v, want %#v", result, containerSvc.networkResult)
	}
	removeErr := errors.New("remove marker")
	containerSvc.errs["RemoveNetwork"] = removeErr
	if err := router.RemoveNetwork(ctx, containerReq); !errors.Is(err, removeErr) {
		t.Fatalf("remove error = %v, want marker", err)
	}
	if err := router.CheckNetwork(ctx, containerReq); err != nil {
		t.Fatalf("check container network: %v", err)
	}
	for _, name := range []string{"SetupNetwork", "RemoveNetwork", "CheckNetwork"} {
		if got := containerSvc.args[name]; !reflect.DeepEqual(got, containerReq) {
			t.Fatalf("%s request = %#v, want %#v", name, got, containerReq)
		}
	}
}

func TestRuntimeRouterRoutesSnapshotsByDriver(t *testing.T) {
	ctx := context.Background()
	containerSvc := newRuntimeRouterRequiredSpy()
	containerSvc.snapshots = []ctr.SnapshotInfo{{Name: "snapshot-a"}}
	local := newRuntimeRouterTestLocal(t, true)
	router := NewRuntimeRouter(containerSvc, local)

	commitReq := ctr.CommitSnapshotRequest{
		Source: ctr.StorageRef{Driver: "overlayfs", Key: "active"},
		Target: ctr.SnapshotRef{Driver: localRuntimeName, Key: "target"},
	}
	commitErr := errors.New("commit marker")
	containerSvc.errs["CommitSnapshot"] = commitErr
	if err := router.CommitSnapshot(ctx, commitReq); !errors.Is(err, commitErr) {
		t.Fatalf("commit error = %v, want marker", err)
	}
	if got := containerSvc.args["CommitSnapshot"]; !reflect.DeepEqual(got, commitReq) {
		t.Fatalf("commit request changed: %#v", got)
	}

	localCommit := ctr.CommitSnapshotRequest{
		Source: ctr.StorageRef{Driver: " local ", Key: "active"},
		Target: ctr.SnapshotRef{Driver: "overlayfs", Key: "target"},
	}
	if err := router.CommitSnapshot(ctx, localCommit); !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("local commit error = %v, want ErrNotSupported", err)
	}

	listReq := ctr.ListSnapshotsRequest{Driver: "overlayfs"}
	listErr := errors.New("list snapshots marker")
	containerSvc.errs["ListSnapshots"] = listErr
	snapshots, err := router.ListSnapshots(ctx, listReq)
	if !errors.Is(err, listErr) {
		t.Fatalf("list snapshots error = %v, want marker", err)
	}
	if !reflect.DeepEqual(snapshots, containerSvc.snapshots) {
		t.Fatalf("snapshots = %#v, want %#v", snapshots, containerSvc.snapshots)
	}
	if got := containerSvc.args["ListSnapshots"]; !reflect.DeepEqual(got, listReq) {
		t.Fatalf("list snapshots request changed: %#v", got)
	}
	if got, err := router.ListSnapshots(ctx, ctr.ListSnapshotsRequest{Driver: " local "}); got != nil || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("local list snapshots = %#v, %v; want nil and ErrNotSupported", got, err)
	}

	prepareReq := ctr.PrepareSnapshotRequest{
		Target: ctr.StorageRef{Driver: "overlayfs", Key: "active"},
		Parent: ctr.SnapshotRef{Driver: localRuntimeName, Key: "parent"},
	}
	if err := router.PrepareSnapshot(ctx, prepareReq); err != nil {
		t.Fatalf("prepare snapshot: %v", err)
	}
	if got := containerSvc.args["PrepareSnapshot"]; !reflect.DeepEqual(got, prepareReq) {
		t.Fatalf("prepare request changed: %#v", got)
	}
	localPrepare := ctr.PrepareSnapshotRequest{
		Target: ctr.StorageRef{Driver: " local ", Key: "active"},
		Parent: ctr.SnapshotRef{Driver: "overlayfs", Key: "parent"},
	}
	if err := router.PrepareSnapshot(ctx, localPrepare); !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("local prepare error = %v, want ErrNotSupported", err)
	}

	restoreReq := ctr.CreateContainerRequest{
		ID:         "memoh-bot-restore",
		StorageRef: ctr.StorageRef{Driver: "overlayfs", Key: "snapshot-a"},
	}
	restored, err := router.RestoreContainer(ctx, restoreReq)
	if err != nil {
		t.Fatalf("restore container: %v", err)
	}
	if restored.ID != restoreReq.ID || !reflect.DeepEqual(containerSvc.args["RestoreContainer"], restoreReq) {
		t.Fatalf("restore request/result changed: result=%#v request=%#v", restored, containerSvc.args["RestoreContainer"])
	}
	localRestore := restoreReq
	localRestore.ID = "local-bot-restore"
	if got, err := router.RestoreContainer(ctx, localRestore); !reflect.DeepEqual(got, ctr.ContainerInfo{}) || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("local restore = %#v, %v; want zero and ErrNotSupported", got, err)
	}

	disabledRouter := NewRuntimeRouter(containerSvc, newRuntimeRouterTestLocal(t, false))
	if err := disabledRouter.CommitSnapshot(ctx, localCommit); !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("disabled local commit error = %v, want ErrNotSupported", err)
	}
	if err := disabledRouter.PrepareSnapshot(ctx, localPrepare); !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("disabled local prepare error = %v, want ErrNotSupported", err)
	}
	beforeRestore := len(containerSvc.calls)
	if _, err := disabledRouter.RestoreContainer(ctx, localRestore); err != nil {
		t.Fatalf("disabled local restore should fall back to container: %v", err)
	}
	if len(containerSvc.calls) != beforeRestore+1 || containerSvc.calls[beforeRestore] != "RestoreContainer" {
		t.Fatalf("disabled local restore calls = %v, want container RestoreContainer", containerSvc.calls[beforeRestore:])
	}
	if got := containerSvc.args["RestoreContainer"]; !reflect.DeepEqual(got, localRestore) {
		t.Fatalf("disabled local restore request changed: %#v", got)
	}
}

func TestRuntimeRouterHandlesOptionalContainerCapabilities(t *testing.T) {
	ctx := context.Background()
	requiredOnly := newRuntimeRouterRequiredSpy()
	router := NewRuntimeRouter(requiredOnly, newRuntimeRouterTestLocal(t, true))

	if got := router.RuntimeType(); got != "" {
		t.Fatalf("runtime type without capability = %q", got)
	}
	if got := router.BridgeTarget("bot-a"); got != "" {
		t.Fatalf("bridge target without capability = %q", got)
	}
	pullOpts := &ctr.PullImageOptions{Unpack: true}
	if got, err := router.PullImage(ctx, "image:a", pullOpts); !reflect.DeepEqual(got, ctr.ImageInfo{}) || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("pull without capability = %#v, %v", got, err)
	}
	if got, err := router.GetImage(ctx, "image:a"); !reflect.DeepEqual(got, ctr.ImageInfo{}) || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("get image without capability = %#v, %v", got, err)
	}
	if got, err := router.ListImages(ctx); got != nil || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("list images without capability = %#v, %v", got, err)
	}
	deleteOpts := &ctr.DeleteImageOptions{Synchronous: true}
	if err := router.DeleteImage(ctx, "image:a", deleteOpts); !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("delete image without capability error = %v", err)
	}
	if digest, err := router.ResolveRemoteDigest(ctx, "image:a"); digest != "" || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("resolve without capability = %q, %v", digest, err)
	}
	if mounts, err := router.SnapshotMounts(ctx, "overlayfs", "active"); mounts != nil || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("mounts without capability = %#v, %v", mounts, err)
	}

	capable := newRuntimeRouterCapabilitySpy()
	router = NewRuntimeRouter(capable, newRuntimeRouterTestLocal(t, true))
	if got := router.RuntimeType(); got != "io.containerd.test.v1" {
		t.Fatalf("runtime type = %q", got)
	}
	if got := router.BridgeTarget("bot-a"); got != "unix:///run/bot-a.sock" {
		t.Fatalf("bridge target = %q", got)
	}
	pullErr := errors.New("pull image marker")
	capable.capabilityErrs["PullImage"] = pullErr
	image, err := router.PullImage(ctx, "image:a", pullOpts)
	if !errors.Is(err, pullErr) || image.ID != "pulled" {
		t.Fatalf("pull image = %#v, %v", image, err)
	}
	if got := capable.args["PullImage"].(struct {
		ref  string
		opts *ctr.PullImageOptions
	}); got.ref != "image:a" || got.opts != pullOpts {
		t.Fatalf("pull arguments changed: %#v", got)
	}
	getErr := errors.New("get image marker")
	capable.capabilityErrs["GetImage"] = getErr
	image, err = router.GetImage(ctx, "image:a")
	if !errors.Is(err, getErr) || image.ID != "present" || capable.args["GetImage"] != "image:a" {
		t.Fatalf("get image = %#v, %v, arg=%#v", image, err, capable.args["GetImage"])
	}
	listErr := errors.New("list images marker")
	capable.capabilityErrs["ListImages"] = listErr
	images, err := router.ListImages(ctx)
	if !errors.Is(err, listErr) || len(images) != 1 || images[0].Name != "image-a" {
		t.Fatalf("list images = %#v, %v", images, err)
	}
	deleteErr := errors.New("delete image marker")
	capable.capabilityErrs["DeleteImage"] = deleteErr
	if err := router.DeleteImage(ctx, "image:a", deleteOpts); !errors.Is(err, deleteErr) {
		t.Fatalf("delete image error = %v, want marker", err)
	}
	if got := capable.args["DeleteImage"].(struct {
		ref  string
		opts *ctr.DeleteImageOptions
	}); got.ref != "image:a" || got.opts != deleteOpts {
		t.Fatalf("delete image arguments changed: %#v", got)
	}
	resolveErr := errors.New("resolve digest marker")
	capable.capabilityErrs["ResolveRemoteDigest"] = resolveErr
	digest, err := router.ResolveRemoteDigest(ctx, "image:a")
	if !errors.Is(err, resolveErr) || digest != "sha256:test" || capable.args["ResolveRemoteDigest"] != "image:a" {
		t.Fatalf("resolve digest = %q, %v, arg=%#v", digest, err, capable.args["ResolveRemoteDigest"])
	}
	mountErr := errors.New("snapshot mounts marker")
	capable.capabilityErrs["SnapshotMounts"] = mountErr
	mounts, err := router.SnapshotMounts(ctx, "overlayfs", "active")
	if !errors.Is(err, mountErr) || len(mounts) != 1 || mounts[0].Source != "/source" {
		t.Fatalf("snapshot mounts = %#v, %v", mounts, err)
	}
	if got := capable.args["SnapshotMounts"]; got != [2]string{"overlayfs", "active"} {
		t.Fatalf("snapshot mounts arguments changed: %#v", got)
	}
	before := len(capable.calls)
	if mounts, err := router.SnapshotMounts(ctx, " local ", "active"); mounts != nil || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("local snapshot mounts = %#v, %v", mounts, err)
	}
	if len(capable.calls) != before {
		t.Fatalf("local snapshot mounts reached container capability: %v", capable.calls[before:])
	}
}

func TestRuntimeRouterResolvesLocalBridgeAndWorkspaceInfo(t *testing.T) {
	ctx := context.Background()
	containerSvc := newRuntimeRouterRequiredSpy()
	local := newRuntimeRouterTestLocal(t, true)
	router := NewRuntimeRouter(containerSvc, local)

	if client, err := router.MCPClient(ctx, "missing"); client != nil || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("missing local MCP = %#v, %v; want nil and ErrNotSupported", client, err)
	}
	info, err := router.WorkspaceInfo(ctx, "missing")
	if err != nil {
		t.Fatalf("missing workspace info: %v", err)
	}
	if info.Backend != bridge.WorkspaceBackendContainer || info.DefaultWorkDir != "/data" {
		t.Fatalf("fallback workspace info = %#v", info)
	}

	corruptID := "local-bot-corrupt"
	if err := os.MkdirAll(local.metadataRoot(), 0o750); err != nil {
		t.Fatalf("create local metadata root: %v", err)
	}
	if err := os.WriteFile(local.metadataPath(corruptID), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt local metadata: %v", err)
	}
	var syntaxErr *json.SyntaxError
	if _, err := router.MCPClient(ctx, "bot-corrupt"); !errors.As(err, &syntaxErr) {
		t.Fatalf("corrupt local MCP error = %v, want json.SyntaxError", err)
	}
	syntaxErr = nil
	if _, err := router.WorkspaceInfo(ctx, "bot-corrupt"); !errors.As(err, &syntaxErr) {
		t.Fatalf("corrupt local workspace info error = %v, want json.SyntaxError", err)
	}

	workspacePath := filepath.Join(t.TempDir(), "workspace")
	createRuntimeRouterLocalContainer(t, local, "local-bot-bridge", nil, workspacePath)
	info, err = router.WorkspaceInfo(ctx, "bot-bridge")
	if err != nil {
		t.Fatalf("local workspace info: %v", err)
	}
	if info.Backend != bridge.WorkspaceBackendLocal || info.DefaultWorkDir != workspacePath {
		t.Fatalf("local workspace info = %#v", info)
	}
	client, err := router.MCPClient(ctx, "bot-bridge")
	if err != nil || client == nil {
		t.Fatalf("local MCP client = %#v, %v", client, err)
	}
	if got := router.DefaultLocalWorkspacePath("bot-bridge", "Display Name"); got == "" {
		t.Fatal("enabled local default workspace path is empty")
	}

	disabledRouter := NewRuntimeRouter(containerSvc, newRuntimeRouterTestLocal(t, false))
	if got := disabledRouter.DefaultLocalWorkspacePath("bot-bridge", "Display Name"); got != "" {
		t.Fatalf("disabled local default workspace path = %q", got)
	}
	if client, err := disabledRouter.MCPClient(ctx, "bot-bridge"); client != nil || !errors.Is(err, ctr.ErrNotSupported) {
		t.Fatalf("disabled local MCP = %#v, %v; want nil and ErrNotSupported", client, err)
	}
}

func newRuntimeRouterTestLocal(t *testing.T, enabled bool) *LocalService {
	t.Helper()
	root := t.TempDir()
	svc := NewLocalService(slog.New(slog.DiscardHandler), config.LocalConfig{
		Enabled:                enabled,
		DefaultWorkspaceParent: filepath.Join(root, "workspaces"),
		MetadataRoot:           filepath.Join(root, "metadata"),
		AllowAbsolutePaths:     true,
	}, filepath.Join(root, "data"))
	t.Cleanup(svc.Close)
	return svc
}

func newBrokenRuntimeRouterTestLocal(t *testing.T) *LocalService {
	t.Helper()
	root := t.TempDir()
	metadataPath := filepath.Join(root, "metadata-file")
	if err := os.WriteFile(metadataPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("create broken metadata fixture: %v", err)
	}
	svc := NewLocalService(slog.New(slog.DiscardHandler), config.LocalConfig{
		Enabled:                true,
		DefaultWorkspaceParent: filepath.Join(root, "workspaces"),
		MetadataRoot:           metadataPath,
	}, filepath.Join(root, "data"))
	t.Cleanup(svc.Close)
	return svc
}

func createRuntimeRouterLocalContainer(t *testing.T, svc *LocalService, id string, labels map[string]string, workspacePath ...string) ctr.ContainerInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workspace")
	if len(workspacePath) > 0 {
		path = workspacePath[0]
	}
	botID := strings.TrimPrefix(id, LocalContainerPrefix)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[BotLabelKey] = botID
	info, err := svc.CreateContainer(context.Background(), ctr.CreateContainerRequest{
		ID:         id,
		ImageRef:   "local",
		StorageRef: ctr.StorageRef{Driver: localRuntimeName, Key: path, Kind: "directory"},
		Labels:     labels,
	})
	if err != nil {
		t.Fatalf("create local fixture %q: %v", id, err)
	}
	return info
}

func containerIDs(items []ctr.ContainerInfo) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func taskContainerIDs(items []ctr.TaskInfo) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ContainerID)
	}
	return out
}
