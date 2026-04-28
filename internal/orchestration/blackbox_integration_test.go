package orchestration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/bcrypt"

	dbembed "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/handlers"
	orch "github.com/memohai/memoh/internal/orchestration"
)

type blackboxHarnessOptions struct {
	startScheduler            bool
	startRecovery             bool
	startVerificationRecovery bool
}

type blackboxHarness struct {
	t           *testing.T
	ctx         context.Context
	cancel      context.CancelFunc
	loopWG      sync.WaitGroup
	server      *http.Server
	listener    net.Listener
	serverErrCh chan error
	baseURL     string
	httpClient  *http.Client

	repoRoot string
	dbName   string
	dbCfg    config.PostgresConfig

	appPool  *pgxpool.Pool
	queries  *sqlc.Queries
	service  *orch.Service
	token    string
	username string
	password string
	secret   string

	plannerOnce              sync.Once
	schedulerOnce            sync.Once
	recoveryOnce             sync.Once
	verificationRecoveryOnce sync.Once

	processMu  sync.Mutex
	processes  []*managedProcess
	configPath string
}

type managedProcess struct {
	name   string
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
}

type blackboxBinarySet struct {
	workerd string
	verifyd string
}

var (
	blackboxBinariesOnce sync.Once
	blackboxBinaries     blackboxBinarySet
	blackboxBinariesErr  error
)

// #nosec G101 -- fixed test-only JWT secret for local blackbox harness.
const blackboxJWTSecret = "memoh-blackbox-test-secret"

func TestBlackboxRuntimeStartRunDispatchExecuteAndComplete(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{
		startScheduler: true,
	})
	defer h.Close()

	h.startWorkerd(t, workerdProcessOptions{
		workerID:   "blackbox-workerd-" + uuid.NewString(),
		profiles:   []string{orch.DefaultRootWorkerProfile},
		pollMS:     50,
		leaseTTL:   2,
		startDelay: 0,
	})

	handle := h.startRun(t, orch.StartRunRequest{
		Goal:           "blackbox happy path run",
		IdempotencyKey: "start-" + uuid.NewString(),
		Input: map[string]any{
			"builtin_workerd": map[string]any{
				"summary": "blackbox happy path completed",
			},
		},
	})

	run := h.waitForRunStatus(t, handle.RunID, orch.LifecycleStatusCompleted, 15*time.Second)
	if run.TerminalReason != "" {
		t.Fatalf("run terminal_reason = %q, want empty", run.TerminalReason)
	}
	task := h.waitForTaskStatus(t, handle.RunID, handle.RootTaskID, orch.TaskStatusCompleted, 5*time.Second)
	if task.LatestResultID == "" {
		t.Fatal("task latest_result_id = empty, want non-empty")
	}
	h.waitForEventType(t, handle.RunID, "run.event.attempt.completed", 5*time.Second)
}

func TestBlackboxRuntimeCheckpointPauseResolveResumeAndComplete(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{})
	defer h.Close()

	handle := h.startRun(t, orch.StartRunRequest{
		Goal:           "checkpoint pause and resume",
		IdempotencyKey: "start-" + uuid.NewString(),
	})

	task := h.waitForTaskStatus(t, handle.RunID, handle.RootTaskID, orch.TaskStatusReady, 10*time.Second)
	checkpoint := h.createCheckpoint(t, handle.RunID, task.ID, orch.CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         task.ID,
		Question:       "continue?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []orch.CheckpointOption{
			{ID: "continue", Kind: orch.CheckpointOptionKindChoice, Label: "Continue"},
		},
	})
	h.waitForTaskStatus(t, handle.RunID, handle.RootTaskID, orch.TaskStatusWaitingHuman, 5*time.Second)
	h.resolveCheckpoint(t, checkpoint.Checkpoint.ID, orch.CheckpointResolution{
		Mode:           orch.CheckpointResolutionModeSelectOption,
		OptionID:       "continue",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	h.waitForTaskStatus(t, handle.RunID, handle.RootTaskID, orch.TaskStatusReady, 5*time.Second)

	h.startSchedulerLoop()
	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-workerd-" + uuid.NewString(),
		profiles: []string{orch.DefaultRootWorkerProfile},
		pollMS:   50,
		leaseTTL: 2,
	})

	h.waitForRunStatus(t, handle.RunID, orch.LifecycleStatusCompleted, 15*time.Second)
	h.waitForEventType(t, handle.RunID, "run.event.task.ready", 5*time.Second)
}

func TestBlackboxRuntimeReplanVerificationAndVerifierCompletion(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{
		startScheduler: true,
	})
	defer h.Close()

	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-workerd-" + uuid.NewString(),
		profiles: []string{orch.DefaultRootWorkerProfile},
		pollMS:   50,
		leaseTTL: 2,
	})
	h.startVerifyd(t, verifydProcessOptions{
		workerID: "blackbox-verifyd-" + uuid.NewString(),
		profiles: []string{orch.DefaultVerifierProfile},
		pollMS:   50,
		leaseTTL: 2,
	})

	handle := h.startRun(t, orch.StartRunRequest{
		Goal:           "blackbox replan verification path",
		IdempotencyKey: "start-" + uuid.NewString(),
		Input: map[string]any{
			"builtin_workerd": map[string]any{
				"request_replan": true,
				"child_tasks": []map[string]any{
					{
						"alias":          "verify-child",
						"kind":           "child",
						"goal":           "verified child",
						"worker_profile": orch.DefaultRootWorkerProfile,
						"verification_policy": map[string]any{
							"require_structured_output": true,
						},
						"inputs": map[string]any{
							"builtin_workerd": map[string]any{
								"summary": "verified child complete",
							},
						},
					},
				},
			},
		},
	})

	h.waitForEventType(t, handle.RunID, "run.event.planner_epoch.advanced", 10*time.Second)
	h.waitForEventType(t, handle.RunID, "run.event.verification.passed", 15*time.Second)
	h.waitForRunStatus(t, handle.RunID, orch.LifecycleStatusCompleted, 20*time.Second)

	taskPage := h.listTasks(t, handle.RunID)
	if len(taskPage.Items) < 2 {
		t.Fatalf("task count = %d, want at least 2", len(taskPage.Items))
	}
}

func TestBlackboxRuntimeClaimedAttemptLeaseExpiryRequeuesAfterWorkerCrash(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{
		startScheduler:            true,
		startRecovery:             true,
		startVerificationRecovery: true,
	})
	defer h.Close()

	slowWorker := h.startWorkerd(t, workerdProcessOptions{
		workerID:   "blackbox-workerd-slow-" + uuid.NewString(),
		profiles:   []string{orch.DefaultRootWorkerProfile},
		pollMS:     50,
		leaseTTL:   2,
		startDelay: 4000,
	})

	handle := h.startRun(t, orch.StartRunRequest{
		Goal:           "claimed attempt should requeue after worker crash",
		IdempotencyKey: "start-" + uuid.NewString(),
	})

	h.waitForEventType(t, handle.RunID, "run.event.attempt.claimed", 10*time.Second)
	slowWorker.stop(t)
	h.waitForEventType(t, handle.RunID, "run.event.attempt.requeued", 10*time.Second)

	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-workerd-fast-" + uuid.NewString(),
		profiles: []string{orch.DefaultRootWorkerProfile},
		pollMS:   50,
		leaseTTL: 2,
	})

	h.waitForRunStatus(t, handle.RunID, orch.LifecycleStatusCompleted, 20*time.Second)
}

func TestBlackboxRuntimeHTTPSnapshotCutsAcrossReadModelsConsistently(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{
		startScheduler: true,
	})
	defer h.Close()

	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-root-workerd-" + uuid.NewString(),
		profiles: []string{orch.DefaultRootWorkerProfile},
		pollMS:   50,
		leaseTTL: 2,
	})

	handle := h.startRun(t, orch.StartRunRequest{
		Goal:           "http snapshot cut should stay consistent across read models",
		IdempotencyKey: "start-" + uuid.NewString(),
		Input: map[string]any{
			"builtin_workerd": map[string]any{
				"request_replan": true,
				"child_tasks": []map[string]any{
					{
						"alias":          "artifact-child",
						"kind":           "child",
						"goal":           "artifact child",
						"worker_profile": "artifact-profile",
						"inputs": map[string]any{
							"builtin_workerd": map[string]any{
								"summary": "artifact child complete",
								"artifact_intents": []map[string]any{
									{
										"kind":         "report",
										"uri":          "memoh://artifact/http-snapshot-cut.md",
										"version":      "v1",
										"digest":       "sha256:http-snapshot-cut",
										"content_type": "text/markdown",
										"summary":      "http snapshot cut artifact",
										"metadata": map[string]any{
											"source": "blackbox-http",
										},
									},
								},
							},
						},
					},
					{
						"alias":          "barrier-child",
						"kind":           "child",
						"goal":           "barrier child",
						"worker_profile": "barrier-profile",
					},
				},
			},
		},
	})

	h.waitForEventType(t, handle.RunID, "run.event.planner_epoch.advanced", 10*time.Second)
	artifactChild := h.waitForTaskGoalStatus(t, handle.RunID, "artifact child", orch.TaskStatusReady, 10*time.Second)
	barrierChild := h.waitForTaskGoalStatus(t, handle.RunID, "barrier child", orch.TaskStatusReady, 10*time.Second)

	checkpoint := h.createCheckpoint(t, handle.RunID, barrierChild.ID, orch.CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         barrierChild.ID,
		Question:       "pause this task?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []orch.CheckpointOption{
			{ID: "resume", Kind: orch.CheckpointOptionKindChoice, Label: "Resume"},
		},
	})
	h.waitForTaskStatus(t, handle.RunID, barrierChild.ID, orch.TaskStatusWaitingHuman, 10*time.Second)

	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-artifact-workerd-" + uuid.NewString(),
		profiles: []string{"artifact-profile"},
		pollMS:   50,
		leaseTTL: 2,
	})

	h.waitForTaskStatus(t, handle.RunID, artifactChild.ID, orch.TaskStatusCompleted, 10*time.Second)
	h.waitForEventType(t, handle.RunID, "run.event.artifact.committed", 10*time.Second)
	snapshotCut := h.getSnapshot(t, handle.RunID).SnapshotSeq

	snapshot := h.getSnapshotAt(t, handle.RunID, snapshotCut)
	taskPage := h.listTasksAt(t, handle.RunID, snapshotCut)
	checkpointPage := h.listCheckpointsAt(t, handle.RunID, snapshotCut)
	artifactPage := h.listArtifactsAt(t, handle.RunID, snapshotCut)
	eventPage := h.listEventsUntil(t, handle.RunID, snapshotCut, 100)

	if snapshot.SnapshotSeq != snapshotCut {
		t.Fatalf("snapshot snapshot_seq = %d, want %d", snapshot.SnapshotSeq, snapshotCut)
	}
	if taskPage.SnapshotSeq != snapshotCut {
		t.Fatalf("task page snapshot_seq = %d, want %d", taskPage.SnapshotSeq, snapshotCut)
	}
	if checkpointPage.SnapshotSeq != snapshotCut {
		t.Fatalf("checkpoint page snapshot_seq = %d, want %d", checkpointPage.SnapshotSeq, snapshotCut)
	}
	if artifactPage.SnapshotSeq != snapshotCut {
		t.Fatalf("artifact page snapshot_seq = %d, want %d", artifactPage.SnapshotSeq, snapshotCut)
	}
	if eventPage.UntilSeq != snapshotCut {
		t.Fatalf("event page until_seq = %d, want %d", eventPage.UntilSeq, snapshotCut)
	}
	if snapshot.Run.LifecycleStatus != orch.LifecycleStatusRunning {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, orch.LifecycleStatusRunning)
	}

	foundBarrierWaiting := false
	foundArtifactCompleted := false
	for _, task := range taskPage.Items {
		switch task.ID {
		case barrierChild.ID:
			if task.Status == orch.TaskStatusWaitingHuman && task.WaitingScope == "task" && task.WaitingCheckpointID == checkpoint.Checkpoint.ID {
				foundBarrierWaiting = true
			}
		case artifactChild.ID:
			if task.Status == orch.TaskStatusCompleted {
				foundArtifactCompleted = true
			}
		}
	}
	if !foundBarrierWaiting {
		t.Fatal("barrier child missing waiting_human view at checkpoint snapshot")
	}
	if !foundArtifactCompleted {
		t.Fatal("artifact child missing completed view at checkpoint snapshot")
	}
	if len(checkpointPage.Items) != 1 || checkpointPage.Items[0].ID != checkpoint.Checkpoint.ID || checkpointPage.Items[0].Status != orch.CheckpointStatusOpen {
		t.Fatalf("checkpoint page = %+v, want single open checkpoint", checkpointPage.Items)
	}
	if len(artifactPage.Items) != 1 || artifactPage.Items[0].Kind != "report" {
		t.Fatalf("artifact page = %+v, want single report artifact", artifactPage.Items)
	}
	for _, event := range eventPage.Items {
		if event.Seq > snapshotCut {
			t.Fatalf("event seq %d exceeds until_seq %d", event.Seq, snapshotCut)
		}
		if event.Type == "run.event.hitl.resolved" {
			t.Fatalf("event page unexpectedly contains future event %q", event.Type)
		}
	}
}

func TestBlackboxRuntimeHTTPIdempotencyAndReplaySafety(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{})
	defer h.Close()

	startKey := "start-" + uuid.NewString()
	startReq := orch.StartRunRequest{
		Goal:           "http idempotency should stay stable",
		IdempotencyKey: startKey,
	}
	handle1 := h.startRun(t, startReq)
	handle2 := h.startRun(t, startReq)
	if handle2.RunID != handle1.RunID || handle2.RootTaskID != handle1.RootTaskID {
		t.Fatalf("start run idempotent retry returned %+v, want %+v", handle2, handle1)
	}

	task := h.waitForTaskStatus(t, handle1.RunID, handle1.RootTaskID, orch.TaskStatusReady, 10*time.Second)
	checkpointKey := "checkpoint-" + uuid.NewString()
	checkpointReq := orch.CreateHumanCheckpointRequest{
		RunID:          handle1.RunID,
		TaskID:         task.ID,
		Question:       "continue?",
		IdempotencyKey: checkpointKey,
		Options: []orch.CheckpointOption{
			{ID: "continue", Kind: orch.CheckpointOptionKindChoice, Label: "Continue"},
		},
	}
	checkpoint1 := h.createCheckpoint(t, handle1.RunID, task.ID, checkpointReq)
	checkpoint2 := h.createCheckpoint(t, handle1.RunID, task.ID, checkpointReq)
	if checkpoint2.Checkpoint.ID != checkpoint1.Checkpoint.ID || checkpoint2.SnapshotSeq != checkpoint1.SnapshotSeq {
		t.Fatalf("checkpoint idempotent retry returned %+v, want %+v", checkpoint2, checkpoint1)
	}

	resolveKey := "resolve-" + uuid.NewString()
	resolveReq := orch.CheckpointResolution{
		Mode:           orch.CheckpointResolutionModeSelectOption,
		OptionID:       "continue",
		IdempotencyKey: resolveKey,
	}
	resolve1 := h.resolveCheckpoint(t, checkpoint1.Checkpoint.ID, resolveReq)
	resolve2 := h.resolveCheckpoint(t, checkpoint1.Checkpoint.ID, resolveReq)
	if resolve2.CheckpointID != resolve1.CheckpointID || resolve2.SnapshotSeq != resolve1.SnapshotSeq {
		t.Fatalf("resolve idempotent retry returned %+v, want %+v", resolve2, resolve1)
	}

	eventPage := h.listEventsUntil(t, handle1.RunID, resolve1.SnapshotSeq, 100)
	if countEventsByType(eventPage.Items, "run.event.created") != 1 {
		t.Fatalf("run.event.created count = %d, want 1", countEventsByType(eventPage.Items, "run.event.created"))
	}
	if countEventsByType(eventPage.Items, "run.event.hitl.requested") != 1 {
		t.Fatalf("run.event.hitl.requested count = %d, want 1", countEventsByType(eventPage.Items, "run.event.hitl.requested"))
	}
	if countEventsByType(eventPage.Items, "run.event.hitl.resolved") != 1 {
		t.Fatalf("run.event.hitl.resolved count = %d, want 1", countEventsByType(eventPage.Items, "run.event.hitl.resolved"))
	}

	page1 := h.listEventsUntil(t, handle1.RunID, resolve1.SnapshotSeq, 2)
	if len(page1.Items) != 2 {
		t.Fatalf("event page1 len = %d, want 2", len(page1.Items))
	}
	page2 := h.listEventsAfterUntil(t, handle1.RunID, page1.Items[len(page1.Items)-1].Seq, resolve1.SnapshotSeq, 100)
	if page2.UntilSeq != page1.UntilSeq {
		t.Fatalf("event page2 until_seq = %d, want %d", page2.UntilSeq, page1.UntilSeq)
	}
	for _, item := range page2.Items {
		if item.Seq <= page1.Items[len(page1.Items)-1].Seq {
			t.Fatalf("event seq %d on page2 should be greater than %d", item.Seq, page1.Items[len(page1.Items)-1].Seq)
		}
		if item.Seq > resolve1.SnapshotSeq {
			t.Fatalf("event seq %d exceeds until_seq %d", item.Seq, resolve1.SnapshotSeq)
		}
	}
}

func TestBlackboxRuntimeHTTPRunBarrierPausesSiblingTasks(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{
		startScheduler: true,
		startRecovery:  true,
	})
	defer h.Close()

	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-root-workerd-" + uuid.NewString(),
		profiles: []string{orch.DefaultRootWorkerProfile},
		pollMS:   50,
		leaseTTL: 2,
	})

	handle := h.startRun(t, orch.StartRunRequest{
		Goal:           "http run barrier should pause sibling tasks",
		IdempotencyKey: "start-" + uuid.NewString(),
		Input: map[string]any{
			"builtin_workerd": map[string]any{
				"request_replan": true,
				"child_tasks": []map[string]any{
					{
						"alias":          "barrier-child",
						"kind":           "child",
						"goal":           "barrier child",
						"worker_profile": "barrier-profile",
					},
					{
						"alias":          "slow-child",
						"kind":           "child",
						"goal":           "slow child",
						"worker_profile": "slow-profile",
						"inputs": map[string]any{
							"builtin_workerd": map[string]any{
								"summary":  "slow child complete",
								"sleep_ms": 4000,
							},
						},
					},
				},
			},
		},
	})

	h.waitForEventType(t, handle.RunID, "run.event.planner_epoch.advanced", 10*time.Second)
	barrierChild := h.waitForTaskGoalStatus(t, handle.RunID, "barrier child", orch.TaskStatusReady, 10*time.Second)
	slowChild := h.waitForTaskGoalStatus(t, handle.RunID, "slow child", orch.TaskStatusReady, 10*time.Second)

	slowWorker := h.startWorkerd(t, workerdProcessOptions{
		workerID:   "blackbox-slow-workerd-" + uuid.NewString(),
		profiles:   []string{"slow-profile"},
		pollMS:     50,
		leaseTTL:   2,
		startDelay: 4000,
	})
	h.waitForEventType(t, handle.RunID, "run.event.attempt.claimed", 10*time.Second)

	checkpoint := h.createCheckpoint(t, handle.RunID, barrierChild.ID, orch.CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         barrierChild.ID,
		Question:       "pause all work?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []orch.CheckpointOption{
			{ID: "resume", Kind: orch.CheckpointOptionKindChoice, Label: "Resume"},
		},
	})

	h.waitForTaskStatus(t, handle.RunID, barrierChild.ID, orch.TaskStatusWaitingHuman, 10*time.Second)
	slowPaused := h.waitForTaskStatus(t, handle.RunID, slowChild.ID, orch.TaskStatusWaitingHuman, 10*time.Second)
	if slowPaused.WaitingScope != "run" {
		t.Fatalf("slow child waiting_scope = %q, want %q", slowPaused.WaitingScope, "run")
	}
	if slowPaused.WaitingCheckpointID != checkpoint.Checkpoint.ID {
		t.Fatalf("slow child waiting_checkpoint_id = %q, want %q", slowPaused.WaitingCheckpointID, checkpoint.Checkpoint.ID)
	}

	resolve := h.resolveCheckpoint(t, checkpoint.Checkpoint.ID, orch.CheckpointResolution{
		Mode:           orch.CheckpointResolutionModeSelectOption,
		OptionID:       "resume",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	slowWorker.stop(t)
	h.waitForEventType(t, handle.RunID, "run.event.hitl.resolved", 10*time.Second)
	h.waitForTaskStatus(t, handle.RunID, barrierChild.ID, orch.TaskStatusReady, 10*time.Second)
	h.waitForTaskStatus(t, handle.RunID, slowChild.ID, orch.TaskStatusReady, 10*time.Second)

	eventPage := h.listEventsUntil(t, handle.RunID, resolve.SnapshotSeq, 100)
	foundResolved := false
	for _, event := range eventPage.Items {
		if event.Type != "run.event.hitl.resolved" {
			continue
		}
		blocksRun, ok := event.Payload["blocks_run"].(bool)
		if !ok || !blocksRun {
			t.Fatalf("resolved event blocks_run payload = %#v, want true", event.Payload["blocks_run"])
		}
		foundResolved = true
	}
	if !foundResolved {
		t.Fatal("missing run.event.hitl.resolved for run barrier")
	}

	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-barrier-workerd-" + uuid.NewString(),
		profiles: []string{"barrier-profile"},
		pollMS:   50,
		leaseTTL: 2,
	})
	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-slow-recovery-workerd-" + uuid.NewString(),
		profiles: []string{"slow-profile"},
		pollMS:   50,
		leaseTTL: 2,
	})
	h.waitForRunStatus(t, handle.RunID, orch.LifecycleStatusCompleted, 20*time.Second)
}

func TestBlackboxRuntimeHTTPVerificationLeaseExpiryRecovery(t *testing.T) {
	h := setupBlackboxHarness(t, blackboxHarnessOptions{
		startScheduler:            true,
		startVerificationRecovery: true,
	})
	defer h.Close()

	h.startWorkerd(t, workerdProcessOptions{
		workerID: "blackbox-workerd-" + uuid.NewString(),
		profiles: []string{orch.DefaultRootWorkerProfile},
		pollMS:   50,
		leaseTTL: 2,
	})

	handle := h.startRun(t, orch.StartRunRequest{
		Goal:           "http verification recovery should requeue claimed verifier work",
		IdempotencyKey: "start-" + uuid.NewString(),
		Input: map[string]any{
			"builtin_workerd": map[string]any{
				"request_replan": true,
				"child_tasks": []map[string]any{
					{
						"alias":          "verify-child",
						"kind":           "child",
						"goal":           "verify child",
						"worker_profile": orch.DefaultRootWorkerProfile,
						"verification_policy": map[string]any{
							"require_structured_output": true,
						},
						"inputs": map[string]any{
							"builtin_workerd": map[string]any{
								"summary": "verify child complete",
							},
						},
					},
				},
			},
		},
	})

	h.waitForEventType(t, handle.RunID, "run.event.planner_epoch.advanced", 10*time.Second)
	slowVerifier := h.startVerifyd(t, verifydProcessOptions{
		workerID:   "blackbox-verifyd-slow-" + uuid.NewString(),
		profiles:   []string{orch.DefaultVerifierProfile},
		pollMS:     50,
		leaseTTL:   2,
		startDelay: 4000,
	})

	h.waitForEventType(t, handle.RunID, "run.event.verification.claimed", 15*time.Second)
	slowVerifier.stop(t)
	h.waitForEventType(t, handle.RunID, "run.event.verification.requeued", 15*time.Second)

	h.startVerifyd(t, verifydProcessOptions{
		workerID: "blackbox-verifyd-fast-" + uuid.NewString(),
		profiles: []string{orch.DefaultVerifierProfile},
		pollMS:   50,
		leaseTTL: 2,
	})

	h.waitForEventType(t, handle.RunID, "run.event.verification.passed", 20*time.Second)
	h.waitForRunStatus(t, handle.RunID, orch.LifecycleStatusCompleted, 20*time.Second)
}

func setupBlackboxHarness(t *testing.T, opts blackboxHarnessOptions) *blackboxHarness {
	t.Helper()

	repoRoot := findRepoRoot(t)
	dbCfg, err := postgresConfigFromTestDSN()
	if err != nil {
		t.Skipf("skip blackbox test: %v", err)
	}

	dbName := "memoh_orch_blackbox_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	adminCfg := dbCfg
	adminCfg.Database = "postgres"
	adminPool, err := db.Open(context.Background(), adminCfg)
	if err != nil {
		t.Skipf("skip blackbox test: open admin db: %v", err)
	}
	if _, err := adminPool.Exec(context.Background(), "CREATE DATABASE "+quoteIdentifier(dbName)); err != nil {
		adminPool.Close()
		t.Skipf("skip blackbox test: create database: %v", err)
	}
	adminPool.Close()

	dbCfg.Database = dbName
	if err := migrateBlackboxDatabase(dbCfg); err != nil {
		dropBlackboxDatabase(t, adminCfg, dbName)
		t.Fatalf("migrate blackbox database: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	appPool, err := db.Open(ctx, dbCfg)
	if err != nil {
		cancel()
		dropBlackboxDatabase(t, adminCfg, dbName)
		t.Fatalf("open app db: %v", err)
	}
	queries := sqlc.New(appPool)
	createBlackboxAdminUser(t, queries, "admin", "admin123", "test@memoh.local")

	logger := slog.New(slog.DiscardHandler)
	service := orch.NewService(logger, appPool, queries)

	listenCfg := net.ListenConfig{}
	listener, err := listenCfg.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		appPool.Close()
		dropBlackboxDatabase(t, adminCfg, dbName)
		t.Fatalf("listen blackbox server: %v", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(auth.JWTMiddleware(blackboxJWTSecret, func(c echo.Context) bool {
		path := c.Request().URL.Path
		return path == "/auth/login" || path == "/ping"
	}))
	e.GET("/ping", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	handlers.NewAuthHandler(logger, accounts.NewService(logger, queries), blackboxJWTSecret, 24*time.Hour).Register(e)
	handlers.NewOrchestrationHandler(logger, service).Register(e)

	serverErrCh := make(chan error, 1)
	server := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
			return
		}
		close(serverErrCh)
	}()

	h := &blackboxHarness{
		t:           t,
		ctx:         ctx,
		cancel:      cancel,
		server:      server,
		listener:    listener,
		serverErrCh: serverErrCh,
		baseURL:     "http://" + listener.Addr().String(),
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		repoRoot:    repoRoot,
		dbName:      dbName,
		dbCfg:       dbCfg,
		appPool:     appPool,
		queries:     queries,
		service:     service,
		username:    "admin",
		password:    "admin123",
		secret:      blackboxJWTSecret,
	}
	h.configPath = writeRuntimeConfig(t, dbCfg)
	h.startPlannerLoop()
	if opts.startScheduler {
		h.startSchedulerLoop()
	}
	if opts.startRecovery {
		h.startRecoveryLoop()
	}
	if opts.startVerificationRecovery {
		h.startVerificationRecoveryLoop()
	}
	h.waitForPing(t)
	h.token = h.login(t)
	return h
}

func (h *blackboxHarness) Close() {
	h.processMu.Lock()
	processes := append([]*managedProcess(nil), h.processes...)
	h.processMu.Unlock()
	for _, process := range processes {
		process.stop(h.t)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.server.Shutdown(shutdownCtx)
	h.cancel()
	done := make(chan struct{})
	go func() {
		h.loopWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		h.t.Fatalf("runtime loops did not stop before timeout")
	}
	h.appPool.Close()
	dropBlackboxDatabase(h.t, config.PostgresConfig{
		Host:     h.dbCfg.Host,
		Port:     h.dbCfg.Port,
		User:     h.dbCfg.User,
		Password: h.dbCfg.Password,
		Database: "postgres",
		SSLMode:  h.dbCfg.SSLMode,
	}, h.dbName)
}

func (h *blackboxHarness) waitForPing(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(h.ctx, http.MethodGet, h.baseURL+"/ping", nil)
		if reqErr != nil {
			t.Fatalf("build ping request: %v", reqErr)
		}
		resp, err := h.httpClient.Do(req)
		if err == nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close ping response body: %v", closeErr)
			}
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case err := <-h.serverErrCh:
			if err != nil {
				t.Fatalf("blackbox server exited early: %v", err)
			}
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("blackbox server did not become ready")
}

func (h *blackboxHarness) startPlannerLoop() {
	h.plannerOnce.Do(func() {
		h.loopWG.Add(1)
		go func() {
			defer h.loopWG.Done()
			h.service.RunPlannerLoop(h.ctx)
		}()
	})
}

func (h *blackboxHarness) startSchedulerLoop() {
	h.schedulerOnce.Do(func() {
		h.loopWG.Add(1)
		go func() {
			defer h.loopWG.Done()
			h.service.RunSchedulerLoop(h.ctx)
		}()
	})
}

func (h *blackboxHarness) startRecoveryLoop() {
	h.recoveryOnce.Do(func() {
		h.loopWG.Add(1)
		go func() {
			defer h.loopWG.Done()
			h.service.RunRecoveryLoop(h.ctx)
		}()
	})
}

func (h *blackboxHarness) startVerificationRecoveryLoop() {
	h.verificationRecoveryOnce.Do(func() {
		h.loopWG.Add(1)
		go func() {
			defer h.loopWG.Done()
			h.service.RunVerificationRecoveryLoop(h.ctx)
		}()
	})
}

type workerdProcessOptions struct {
	workerID       string
	profiles       []string
	pollMS         int
	leaseTTL       int
	startDelay     int
	executionDelay int
}

func (h *blackboxHarness) startWorkerd(t *testing.T, opts workerdProcessOptions) *managedProcess {
	t.Helper()
	binaries := buildBlackboxBinaries(t)
	env := []string{
		"CONFIG_PATH=" + h.configPath,
		"WORKER_ID=" + opts.workerID,
		"WORKER_PROFILES=" + strings.Join(opts.profiles, ","),
		fmt.Sprintf("WORKER_POLL_INTERVAL_MS=%d", choosePositive(opts.pollMS, 50)),
		fmt.Sprintf("WORKER_LEASE_TTL_SECONDS=%d", choosePositive(opts.leaseTTL, 2)),
		fmt.Sprintf("WORKER_START_DELAY_MS=%d", maxInt(opts.startDelay, 0)),
		fmt.Sprintf("WORKER_EXECUTION_DELAY_MS=%d", maxInt(opts.executionDelay, 0)),
	}
	return h.startManagedProcess(t, "workerd", binaries.workerd, env)
}

type verifydProcessOptions struct {
	workerID       string
	profiles       []string
	pollMS         int
	leaseTTL       int
	startDelay     int
	executionDelay int
}

func (h *blackboxHarness) startVerifyd(t *testing.T, opts verifydProcessOptions) *managedProcess {
	t.Helper()
	binaries := buildBlackboxBinaries(t)
	env := []string{
		"CONFIG_PATH=" + h.configPath,
		"VERIFIER_ID=" + opts.workerID,
		"VERIFIER_PROFILES=" + strings.Join(opts.profiles, ","),
		fmt.Sprintf("VERIFIER_POLL_INTERVAL_MS=%d", choosePositive(opts.pollMS, 50)),
		fmt.Sprintf("VERIFIER_LEASE_TTL_SECONDS=%d", choosePositive(opts.leaseTTL, 2)),
		fmt.Sprintf("VERIFIER_START_DELAY_MS=%d", maxInt(opts.startDelay, 0)),
		fmt.Sprintf("VERIFIER_EXECUTION_DELAY_MS=%d", maxInt(opts.executionDelay, 0)),
	}
	return h.startManagedProcess(t, "verifyd", binaries.verifyd, env)
}

func (h *blackboxHarness) startManagedProcess(t *testing.T, name, binary string, env []string) *managedProcess {
	t.Helper()
	cmd := exec.CommandContext(h.ctx, binary)
	cmd.Dir = h.repoRoot
	cmd.Env = append(os.Environ(), env...)
	process := &managedProcess{name: name, cmd: cmd}
	cmd.Stdout = &process.stdout
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	h.processMu.Lock()
	h.processes = append(h.processes, process)
	h.processMu.Unlock()
	return process
}

func (p *managedProcess) stop(t *testing.T) {
	t.Helper()
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if p.cmd.ProcessState == nil || !p.cmd.ProcessState.Exited() {
		_ = p.cmd.Process.Kill()
	}
	_ = p.cmd.Wait()
}

func (h *blackboxHarness) login(t *testing.T) string {
	t.Helper()
	body := handlers.LoginRequest{
		Username: h.username,
		Password: h.password,
	}
	var resp handlers.LoginResponse
	h.mustJSON(t, http.MethodPost, "/auth/login", body, "", &resp, http.StatusOK)
	if strings.TrimSpace(resp.AccessToken) == "" {
		t.Fatal("login token = empty")
	}
	return resp.AccessToken
}

func (h *blackboxHarness) startRun(t *testing.T, req orch.StartRunRequest) orch.RunHandle {
	t.Helper()
	var handle orch.RunHandle
	h.mustJSON(t, http.MethodPost, "/orchestration/runs", req, h.token, &handle, http.StatusCreated)
	if handle.RunID == "" || handle.RootTaskID == "" {
		t.Fatalf("invalid run handle: %+v", handle)
	}
	return handle
}

func (h *blackboxHarness) createCheckpoint(t *testing.T, runID, taskID string, req orch.CreateHumanCheckpointRequest) orch.CreateHumanCheckpointResult {
	t.Helper()
	var result orch.CreateHumanCheckpointResult
	h.mustJSON(t, http.MethodPost, "/orchestration/runs/"+runID+"/tasks/"+taskID+"/checkpoints", req, h.token, &result, http.StatusCreated)
	return result
}

func (h *blackboxHarness) resolveCheckpoint(t *testing.T, checkpointID string, req orch.CheckpointResolution) orch.ResolveCheckpointResult {
	t.Helper()
	var result orch.ResolveCheckpointResult
	h.mustJSON(t, http.MethodPost, "/orchestration/checkpoints/"+checkpointID+"/resolve", req, h.token, &result, http.StatusOK)
	return result
}

func (h *blackboxHarness) listTasks(t *testing.T, runID string) orch.TaskPage {
	t.Helper()
	var page orch.TaskPage
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/tasks?limit=100", nil, h.token, &page, http.StatusOK)
	return page
}

func (h *blackboxHarness) getSnapshot(t *testing.T, runID string) orch.RunSnapshot {
	t.Helper()
	var snapshot orch.RunSnapshot
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/snapshot", nil, h.token, &snapshot, http.StatusOK)
	return snapshot
}

func (h *blackboxHarness) getSnapshotAt(t *testing.T, runID string, asOfSeq uint64) orch.RunSnapshot {
	t.Helper()
	var snapshot orch.RunSnapshot
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/snapshot?as_of_seq="+strconv.FormatUint(asOfSeq, 10), nil, h.token, &snapshot, http.StatusOK)
	return snapshot
}

func (h *blackboxHarness) listEvents(t *testing.T, runID string) orch.RunEventPage {
	t.Helper()
	var page orch.RunEventPage
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/events?limit=500", nil, h.token, &page, http.StatusOK)
	return page
}

func (h *blackboxHarness) listTasksAt(t *testing.T, runID string, asOfSeq uint64) orch.TaskPage {
	t.Helper()
	query := url.Values{}
	query.Set("limit", "100")
	query.Set("as_of_seq", strconv.FormatUint(asOfSeq, 10))
	var page orch.TaskPage
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/tasks?"+query.Encode(), nil, h.token, &page, http.StatusOK)
	return page
}

func (h *blackboxHarness) listCheckpointsAt(t *testing.T, runID string, asOfSeq uint64) orch.HumanCheckpointPage {
	t.Helper()
	query := url.Values{}
	query.Set("limit", "100")
	query.Set("as_of_seq", strconv.FormatUint(asOfSeq, 10))
	var page orch.HumanCheckpointPage
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/checkpoints?"+query.Encode(), nil, h.token, &page, http.StatusOK)
	return page
}

func (h *blackboxHarness) listArtifactsAt(t *testing.T, runID string, asOfSeq uint64) orch.ArtifactPage {
	t.Helper()
	query := url.Values{}
	query.Set("limit", "100")
	query.Set("as_of_seq", strconv.FormatUint(asOfSeq, 10))
	var page orch.ArtifactPage
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/artifacts?"+query.Encode(), nil, h.token, &page, http.StatusOK)
	return page
}

func (h *blackboxHarness) listEventsUntil(t *testing.T, runID string, untilSeq uint64, limit int) orch.RunEventPage {
	t.Helper()
	query := url.Values{}
	query.Set("limit", strconv.Itoa(choosePositive(limit, 100)))
	query.Set("until_seq", strconv.FormatUint(untilSeq, 10))
	var page orch.RunEventPage
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/events?"+query.Encode(), nil, h.token, &page, http.StatusOK)
	return page
}

func (h *blackboxHarness) listEventsAfterUntil(t *testing.T, runID string, afterSeq, untilSeq uint64, limit int) orch.RunEventPage {
	t.Helper()
	query := url.Values{}
	query.Set("limit", strconv.Itoa(choosePositive(limit, 100)))
	query.Set("after_seq", strconv.FormatUint(afterSeq, 10))
	query.Set("until_seq", strconv.FormatUint(untilSeq, 10))
	var page orch.RunEventPage
	h.mustJSON(t, http.MethodGet, "/orchestration/runs/"+runID+"/events?"+query.Encode(), nil, h.token, &page, http.StatusOK)
	return page
}

func (h *blackboxHarness) waitForRunStatus(t *testing.T, runID, status string, timeout time.Duration) orch.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := h.getSnapshot(t, runID)
		if snapshot.Run.LifecycleStatus == status {
			return snapshot.Run
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach status %q within %s", runID, status, timeout)
	return orch.Run{}
}

func (h *blackboxHarness) waitForTaskStatus(t *testing.T, runID, taskID, status string, timeout time.Duration) orch.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		page := h.listTasks(t, runID)
		for _, task := range page.Items {
			if task.ID == taskID && task.Status == status {
				return task
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach status %q within %s", taskID, status, timeout)
	return orch.Task{}
}

func (h *blackboxHarness) waitForTaskGoalStatus(t *testing.T, runID, goal, status string, timeout time.Duration) orch.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		page := h.listTasks(t, runID)
		for _, task := range page.Items {
			if task.Goal == goal && task.Status == status {
				return task
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("task goal %q did not reach status %q within %s", goal, status, timeout)
	return orch.Task{}
}

func (h *blackboxHarness) waitForEventType(t *testing.T, runID, eventType string, timeout time.Duration) orch.RunEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		page := h.listEvents(t, runID)
		for _, event := range page.Items {
			if event.Type == eventType {
				return event
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s did not emit event %q within %s", runID, eventType, timeout)
	return orch.RunEvent{}
}

func (h *blackboxHarness) mustJSON(t *testing.T, method, path string, payload any, token string, dest any, wantStatus int) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal %s %s payload: %v", method, path, err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(h.ctx, method, h.baseURL+path, body)
	if err != nil {
		t.Fatalf("build %s %s request: %v", method, path, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s request: %v", method, path, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close %s %s response body: %v", method, path, closeErr)
		}
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s response: %v", method, path, err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, resp.StatusCode, wantStatus, strings.TrimSpace(string(raw)))
	}
	if dest == nil || len(raw) == 0 {
		return
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, strings.TrimSpace(string(raw)))
	}
}

func buildBlackboxBinaries(t *testing.T) blackboxBinarySet {
	t.Helper()
	blackboxBinariesOnce.Do(func() {
		repoRoot := findRepoRoot(t)
		dir, err := os.MkdirTemp(repoRoot, ".memoh-orchestration-blackbox-bin-*")
		if err != nil {
			blackboxBinariesErr = err
			return
		}
		workerd := filepath.Join(dir, "workerd")
		verifyd := filepath.Join(dir, "verifyd")
		if err := runGoBuild(repoRoot, workerd, "./cmd/workerd"); err != nil {
			blackboxBinariesErr = err
			return
		}
		if err := runGoBuild(repoRoot, verifyd, "./cmd/verifyd"); err != nil {
			blackboxBinariesErr = err
			return
		}
		blackboxBinaries = blackboxBinarySet{
			workerd: workerd,
			verifyd: verifyd,
		}
	})
	if blackboxBinariesErr != nil {
		t.Fatalf("build blackbox binaries: %v", blackboxBinariesErr)
	}
	return blackboxBinaries
}

func runGoBuild(repoRoot, out, pkg string) error {
	// #nosec G204 -- test harness only builds repository-local packages into a temp dir.
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", out, pkg)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/go-build")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build %s: %w\n%s", pkg, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func postgresConfigFromTestDSN() (config.PostgresConfig, error) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		return config.PostgresConfig{}, errors.New("TEST_POSTGRES_DSN is not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return config.PostgresConfig{}, fmt.Errorf("parse TEST_POSTGRES_DSN: %w", err)
	}
	return config.PostgresConfig{
		Host:     cfg.ConnConfig.Host,
		Port:     int(cfg.ConnConfig.Port),
		User:     cfg.ConnConfig.User,
		Password: cfg.ConnConfig.Password,
		Database: cfg.ConnConfig.Database,
		SSLMode:  chooseString(cfg.ConnConfig.RuntimeParams["sslmode"], "disable"),
	}, nil
}

func migrateBlackboxDatabase(dbCfg config.PostgresConfig) error {
	sub, err := fs.Sub(dbembed.MigrationsFS, "migrations")
	if err != nil {
		return err
	}
	return db.RunMigrate(slog.New(slog.DiscardHandler), dbCfg, sub, "up", nil)
}

func createBlackboxAdminUser(t *testing.T, queries *sqlc.Queries, username, password, email string) {
	t.Helper()
	ctx := context.Background()
	count, err := queries.CountAccounts(ctx)
	if err != nil {
		t.Fatalf("CountAccounts() error = %v", err)
	}
	if count > 0 {
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	user, err := queries.CreateUser(ctx, sqlc.CreateUserParams{
		IsActive: true,
		Metadata: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	_, err = queries.CreateAccount(ctx, sqlc.CreateAccountParams{
		UserID:       user.ID,
		Username:     pgtype.Text{String: username, Valid: true},
		Email:        pgtype.Text{String: email, Valid: true},
		PasswordHash: pgtype.Text{String: string(hashed), Valid: true},
		Role:         "admin",
		DisplayName:  pgtype.Text{String: username, Valid: true},
		AvatarUrl:    pgtype.Text{},
		IsActive:     true,
		DataRoot:     pgtype.Text{String: "data", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
}

func dropBlackboxDatabase(t *testing.T, adminCfg config.PostgresConfig, dbName string) {
	t.Helper()
	pool, err := db.Open(context.Background(), adminCfg)
	if err != nil {
		t.Fatalf("open admin db for cleanup: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), `
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = $1
  AND pid <> pg_backend_pid()
`, dbName); err != nil {
		t.Fatalf("terminate blackbox db sessions: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+quoteIdentifier(dbName)); err != nil {
		t.Fatalf("drop blackbox database: %v", err)
	}
}

func writeRuntimeConfig(t *testing.T, dbCfg config.PostgresConfig) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "orchestration-blackbox.toml")
	content := fmt.Sprintf(`
[log]
level = "info"
format = "text"

[postgres]
host = %q
port = %d
user = %q
password = %q
database = %q
sslmode = %q
`, dbCfg.Host, dbCfg.Port, dbCfg.User, dbCfg.Password, dbCfg.Database, chooseString(dbCfg.SSLMode, "disable"))
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}
	return path
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func choosePositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func maxInt(value, floor int) int {
	if value < floor {
		return floor
	}
	return value
}

func chooseString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func countEventsByType(items []orch.RunEvent, eventType string) int {
	count := 0
	for _, item := range items {
		if item.Type == eventType {
			count++
		}
	}
	return count
}
