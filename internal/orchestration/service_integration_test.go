package orchestration

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

func setupOrchestrationIntegrationTest(t *testing.T) (*Service, *pgxpool.Pool, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip integration test: TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip integration test: cannot connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skip integration test: database ping failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
ALTER TABLE IF EXISTS orchestration_task_attempts
  ADD COLUMN IF NOT EXISTS worker_lease_token TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS orchestration_task_verifications
  ADD COLUMN IF NOT EXISTS worker_lease_token TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_orchestration_human_checkpoints_open_run_barrier_unique
  ON orchestration_human_checkpoints (run_id)
  WHERE blocks_run = TRUE AND status = 'open';
`); err != nil {
		pool.Close()
		t.Skipf("skip integration test: database schema bootstrap failed: %v", err)
	}

	logger := slog.New(slog.DiscardHandler)
	return NewService(logger, pool, sqlc.New(pool)), pool, func() { pool.Close() }
}

func cleanupOrchestrationIntegrationRun(t *testing.T, ctx context.Context, pool *pgxpool.Pool, runID string) {
	t.Helper()

	pgRunID, err := db.ParseUUID(runID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM orchestration_runs WHERE id = $1", pgRunID); err != nil {
		t.Fatalf("delete orchestration run: %v", err)
	}
}

func cleanupOrchestrationIntegrationIdempotency(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, subject string) {
	t.Helper()

	if _, err := pool.Exec(ctx, "DELETE FROM orchestration_idempotency_records WHERE tenant_id = $1 AND caller_subject = $2", tenantID, subject); err != nil {
		t.Fatalf("delete orchestration idempotency records: %v", err)
	}
}

func setIntegrationTaskVerificationPolicy(t *testing.T, ctx context.Context, pool *pgxpool.Pool, taskID string, policy map[string]any) {
	t.Helper()

	pgTaskID, err := db.ParseUUID(taskID)
	if err != nil {
		t.Fatalf("parse task uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_tasks SET verification_policy = $2::jsonb, updated_at = now() WHERE id = $1", pgTaskID, marshalJSON(policy)); err != nil {
		t.Fatalf("update task verification policy: %v", err)
	}
}

func processRunPlanningIntent(t *testing.T, ctx context.Context, svc *Service) {
	t.Helper()

	processed, err := svc.ProcessNextPlanningIntent(ctx)
	if err != nil {
		t.Fatalf("ProcessNextPlanningIntent() error = %v", err)
	}
	if !processed {
		t.Fatal("ProcessNextPlanningIntent() = false, want true")
	}
}

func createIntegrationChildTask(t *testing.T, ctx context.Context, svc *Service, runID string, goal string, blackboardScope string) string {
	t.Helper()

	pgRunID, err := db.ParseUUID(runID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	runRow, err := svc.queries.GetOrchestrationRunByID(ctx, pgRunID)
	if err != nil {
		t.Fatalf("GetOrchestrationRunByID() error = %v", err)
	}
	rawTaskID, taskUUID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(child task) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   taskUUID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 goal,
		Inputs:               marshalObject(map[string]any{"goal": goal}),
		PlannerEpoch:         runRow.PlannerEpoch,
		WorkerProfile:        DefaultRootWorkerProfile,
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusReady,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      blackboardScope,
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(%s) error = %v", goal, err)
	}
	return rawTaskID
}

func drainRunPlanningIntents(t *testing.T, ctx context.Context, svc *Service) {
	t.Helper()

	for {
		processed, err := svc.ProcessNextPlanningIntent(ctx)
		if err != nil {
			t.Fatalf("ProcessNextPlanningIntent() error = %v", err)
		}
		if !processed {
			return
		}
	}
}

func dispatchAndClaimAttemptForProfiles(t *testing.T, ctx context.Context, svc *Service, claim AttemptClaim, maxDispatches int) *TaskAttempt {
	t.Helper()

	if strings.TrimSpace(claim.LeaseToken) == "" {
		claim.LeaseToken = "lease-" + uuid.NewString()
	}

	for i := 0; i < maxDispatches; i++ {
		attempt, err := svc.ClaimNextAttempt(ctx, claim)
		if err == nil {
			return attempt
		}
		if !errors.Is(err, ErrNoRunnableAttempt) {
			t.Fatalf("ClaimNextAttempt() error = %v", err)
		}
		dispatched, dispatchErr := svc.DispatchNextReadyTask(ctx)
		if dispatchErr != nil {
			t.Fatalf("DispatchNextReadyTask() error = %v", dispatchErr)
		}
		if !dispatched {
			break
		}
	}

	attempt, err := svc.ClaimNextAttempt(ctx, claim)
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	return attempt
}

func TestIntegrationTenantScopedAuthorizationRejectsSameSubjectAcrossTenants(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	subject := "shared-subject-" + uuid.NewString()
	caller := ControlIdentity{
		TenantID: "tenant-a-" + uuid.NewString(),
		Subject:  subject,
	}
	otherOwnerCaller := ControlIdentity{
		TenantID: caller.TenantID,
		Subject:  "subject-" + uuid.NewString(),
	}
	otherTenantCaller := ControlIdentity{
		TenantID: "tenant-b-" + uuid.NewString(),
		Subject:  subject,
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "authorize only within the owning tenant",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, otherOwnerCaller.TenantID, otherOwnerCaller.Subject)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, otherTenantCaller.TenantID, otherTenantCaller.Subject)

	if _, err := svc.GetRunSnapshot(ctx, otherTenantCaller, handle.RunID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("GetRunSnapshot(cross-tenant same-subject) error = %v, want %v", err, ErrRunNotFound)
	}
	if _, err := svc.GetRunSnapshot(ctx, otherOwnerCaller, handle.RunID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("GetRunSnapshot(same-tenant non-owner) error = %v, want %v", err, ErrRunNotFound)
	}

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "approve execution?",
		BlocksRun:      false,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "approve", Kind: CheckpointOptionKindChoice, Label: "Approve"},
		},
		Metadata: map[string]any{"context": "tenant-authorization"},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}

	_, err = svc.ResolveCheckpoint(ctx, otherTenantCaller, checkpointResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "approve",
		Metadata:       map[string]any{"reviewer": "bob"},
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	if !errors.Is(err, ErrCheckpointNotFound) {
		t.Fatalf("ResolveCheckpoint(cross-tenant same-subject) error = %v, want %v", err, ErrCheckpointNotFound)
	}
	_, err = svc.ResolveCheckpoint(ctx, otherOwnerCaller, checkpointResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "approve",
		Metadata:       map[string]any{"reviewer": "carol"},
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	if !errors.Is(err, ErrCheckpointNotFound) {
		t.Fatalf("ResolveCheckpoint(same-tenant non-owner) error = %v, want %v", err, ErrCheckpointNotFound)
	}

	page, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("ListRunCheckpoints() len = %d, want 1", len(page.Items))
	}
	if page.Items[0].Status != CheckpointStatusOpen {
		t.Fatalf("checkpoint status after denied resolve = %q, want %q", page.Items[0].Status, CheckpointStatusOpen)
	}
}

func TestIntegrationResolveCheckpointPreservesCheckpointMetadata(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "preserve checkpoint metadata on resolve",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	checkpointMetadata := map[string]any{
		"context": "approval gate",
		"source":  "integration-test",
	}
	resolutionMetadata := map[string]any{
		"reviewer": "alice",
		"reason":   "looks good",
	}

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "ship this change?",
		BlocksRun:      false,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ship", Kind: CheckpointOptionKindChoice, Label: "Ship"},
		},
		Metadata: checkpointMetadata,
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}

	if _, err := svc.ResolveCheckpoint(ctx, caller, checkpointResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "ship",
		Metadata:       resolutionMetadata,
		IdempotencyKey: "resolve-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	page, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("ListRunCheckpoints() len = %d, want 1", len(page.Items))
	}
	if !reflect.DeepEqual(page.Items[0].Metadata, checkpointMetadata) {
		t.Fatalf("checkpoint metadata after resolve = %#v, want %#v", page.Items[0].Metadata, checkpointMetadata)
	}

	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	var found bool
	for _, event := range eventPage.Items {
		if event.Type != "run.event.hitl.resolved" {
			continue
		}
		blocksRun, ok := event.Payload["blocks_run"].(bool)
		if !ok {
			t.Fatalf("resolved event blocks_run type = %T, want bool", event.Payload["blocks_run"])
		}
		if blocksRun {
			t.Fatal("resolved event blocks_run = true, want false")
		}
		got, ok := event.Payload["resolution_metadata"].(map[string]any)
		if !ok {
			t.Fatalf("resolved event resolution_metadata type = %T, want map[string]any", event.Payload["resolution_metadata"])
		}
		if !reflect.DeepEqual(got, resolutionMetadata) {
			t.Fatalf("resolved event resolution_metadata = %#v, want %#v", got, resolutionMetadata)
		}
		found = true
	}
	if !found {
		t.Fatal("ListRunEvents() missing run.event.hitl.resolved")
	}
}

func TestIntegrationCreateHumanCheckpointHidesForeignTaskOnAuthorizedRun(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	firstHandle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "first run",
		IdempotencyKey: "start-a-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun(first) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, firstHandle.RunID)

	secondHandle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "second run",
		IdempotencyKey: "start-b-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun(second) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, secondHandle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          firstHandle.RunID,
		TaskID:         secondHandle.RootTaskID,
		Question:       "should stay hidden?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("CreateHumanCheckpoint(foreign task under run) error = %v, want %v", err, ErrTaskNotFound)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsSecondActiveCheckpointForTask(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "prevent replacing an active task wait with a later checkpoint",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	firstCheckpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "first approval?",
		BlocksRun:      false,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "approve", Kind: CheckpointOptionKindChoice, Label: "Approve"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(first) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "second approval?",
		BlocksRun:      false,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "approve", Kind: CheckpointOptionKindChoice, Label: "Approve"},
		},
	})
	if !errors.Is(err, ErrTaskAlreadyWaitingHuman) {
		t.Fatalf("CreateHumanCheckpoint(second) error = %v, want %v", err, ErrTaskAlreadyWaitingHuman)
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 {
		t.Fatalf("ListRunTasks() len = %d, want 1", len(taskPage.Items))
	}
	if taskPage.Items[0].WaitingCheckpointID != firstCheckpointResult.Checkpoint.ID {
		t.Fatalf("waiting_checkpoint_id = %q, want %q", taskPage.Items[0].WaitingCheckpointID, firstCheckpointResult.Checkpoint.ID)
	}

	checkpointPage, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(checkpointPage.Items) != 1 {
		t.Fatalf("ListRunCheckpoints() len = %d, want 1", len(checkpointPage.Items))
	}
	if checkpointPage.Items[0].ID != firstCheckpointResult.Checkpoint.ID {
		t.Fatalf("remaining checkpoint id = %q, want %q", checkpointPage.Items[0].ID, firstCheckpointResult.Checkpoint.ID)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsTerminalTask(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "do not resurrect terminal tasks with hitl",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgTaskID, err := db.ParseUUID(handle.RootTaskID)
	if err != nil {
		t.Fatalf("parse task uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE orchestration_tasks
		SET status = 'completed',
		    status_version = status_version + 1,
		    terminal_reason = 'done',
		    updated_at = now()
		WHERE id = $1
	`, pgTaskID); err != nil {
		t.Fatalf("mark task completed: %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "approve after completion?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "approve", Kind: CheckpointOptionKindChoice, Label: "Approve"},
		},
	})
	if !errors.Is(err, ErrTaskImmutable) {
		t.Fatalf("CreateHumanCheckpoint(terminal task) error = %v, want %v", err, ErrTaskImmutable)
	}

	var status string
	var waitingCheckpointID *string
	if err := pool.QueryRow(ctx, `
		SELECT status, waiting_checkpoint_id::text
		FROM orchestration_tasks
		WHERE id = $1
	`, pgTaskID).Scan(&status, &waitingCheckpointID); err != nil {
		t.Fatalf("load task after rejected checkpoint: %v", err)
	}
	if status != TaskStatusCompleted {
		t.Fatalf("task status after rejected checkpoint = %q, want %q", status, TaskStatusCompleted)
	}
	if waitingCheckpointID != nil && strings.TrimSpace(*waitingCheckpointID) != "" {
		t.Fatalf("waiting_checkpoint_id after rejected checkpoint = %q, want empty", *waitingCheckpointID)
	}

	var checkpointCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM orchestration_human_checkpoints
		WHERE task_id = $1
	`, pgTaskID).Scan(&checkpointCount); err != nil {
		t.Fatalf("count checkpoints after rejected create: %v", err)
	}
	if checkpointCount != 0 {
		t.Fatalf("checkpoint count after rejected create = %d, want 0", checkpointCount)
	}
}

func TestIntegrationStartRunLaunchesReadyRootTask(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "launch root task immediately",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusRunning {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusRunning)
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 {
		t.Fatalf("ListRunTasks() len = %d, want 1", len(taskPage.Items))
	}
	if taskPage.Items[0].Status != TaskStatusReady {
		t.Fatalf("root task status = %q, want %q", taskPage.Items[0].Status, TaskStatusReady)
	}
}

func TestIntegrationSchedulerAndAttemptLifecycleCompletesRootTask(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "execute root task through attempt lifecycle",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}

	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	completedAttempt, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "root task completed",
		StructuredOutput: map[string]any{"ok": true},
	})
	if err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	if completedAttempt.Status != TaskAttemptStatusCompleted {
		t.Fatalf("completed attempt status = %q, want %q", completedAttempt.Status, TaskAttemptStatusCompleted)
	}
	processRunPlanningIntent(t, ctx, svc)

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusCompleted {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusCompleted)
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 {
		t.Fatalf("ListRunTasks() len = %d, want 1", len(taskPage.Items))
	}
	if taskPage.Items[0].Status != TaskStatusCompleted {
		t.Fatalf("root task status = %q, want %q", taskPage.Items[0].Status, TaskStatusCompleted)
	}

	pgAttemptID, err := db.ParseUUID(completedAttempt.ID)
	if err != nil {
		t.Fatalf("parse completed attempt id: %v", err)
	}
	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusCompleted {
		t.Fatalf("attempt row status = %q, want %q", attemptRow.Status, TaskAttemptStatusCompleted)
	}
}

func TestIntegrationAttemptCompletionRequestReplanCreatesReadyChildTasks(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "expand child tasks from request_replan output",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}

	attempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	}, 4)
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningAttempt.ID,
		ClaimToken:    runningAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "root task planned child tasks",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{
					"goal":             "child task one",
					"worker_profile":   DefaultRootWorkerProfile,
					"inputs":           map[string]any{"step": 1},
					"blackboard_scope": "run.child.one",
				},
				map[string]any{
					"goal": "child task two",
				},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(request_replan) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	intermediateSnapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(intermediate) error = %v", err)
	}
	if intermediateSnapshot.Run.PlanningStatus != PlanningStatusActive {
		t.Fatalf("intermediate planning_status = %q, want %q", intermediateSnapshot.Run.PlanningStatus, PlanningStatusActive)
	}
	intermediateTasks, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(intermediate) error = %v", err)
	}
	if len(intermediateTasks.Items) != 1 {
		t.Fatalf("intermediate task count = %d, want 1 before replan intent runs", len(intermediateTasks.Items))
	}
	if intermediateTasks.Items[0].ID != handle.RootTaskID || intermediateTasks.Items[0].Status != TaskStatusCompleted {
		t.Fatalf("intermediate root task = %+v, want completed root only", intermediateTasks.Items[0])
	}
	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	var pendingReplanCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM orchestration_planning_intents
		WHERE run_id = $1
		  AND kind = $2
		  AND status = $3
	`, pgRunID, PlanningIntentKindReplan, PlanningIntentStatusPending).Scan(&pendingReplanCount); err != nil {
		t.Fatalf("query pending replan intents: %v", err)
	}
	if pendingReplanCount != 1 {
		t.Fatalf("pending replan intent count = %d, want 1", pendingReplanCount)
	}

	processRunPlanningIntent(t, ctx, svc)

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusRunning {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusRunning)
	}
	if snapshot.Run.PlanningStatus != PlanningStatusIdle {
		t.Fatalf("run planning_status = %q, want %q", snapshot.Run.PlanningStatus, PlanningStatusIdle)
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 3 {
		t.Fatalf("ListRunTasks() len = %d, want 3", len(taskPage.Items))
	}
	readyChildren := 0
	for _, task := range taskPage.Items {
		if task.ID == handle.RootTaskID {
			if task.Status != TaskStatusCompleted {
				t.Fatalf("root task status = %q, want %q", task.Status, TaskStatusCompleted)
			}
			continue
		}
		if task.Status != TaskStatusReady {
			t.Fatalf("child task %q status = %q, want %q", task.ID, task.Status, TaskStatusReady)
		}
		if task.DecomposedFromTaskID != handle.RootTaskID {
			t.Fatalf("child task %q decomposed_from_task_id = %q, want %q", task.ID, task.DecomposedFromTaskID, handle.RootTaskID)
		}
		readyChildren++
	}
	if readyChildren != 2 {
		t.Fatalf("ready child task count = %d, want 2", readyChildren)
	}
}

func TestIntegrationAttemptCompletionWithVerificationPassesRun(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "verify task output before completing run",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}

	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "root task produced structured output",
		StructuredOutput: map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(verifying) error = %v", err)
	}
	if len(taskPage.Items) != 1 {
		t.Fatalf("ListRunTasks(verifying) len = %d, want 1", len(taskPage.Items))
	}
	if taskPage.Items[0].Status != TaskStatusVerifying {
		t.Fatalf("root task status = %q, want %q", taskPage.Items[0].Status, TaskStatusVerifying)
	}

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}
	completedVerification, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "verified",
	})
	if err != nil {
		t.Fatalf("CompleteVerification() error = %v", err)
	}
	if completedVerification.Status != TaskVerificationStatusCompleted {
		t.Fatalf("verification status = %q, want %q", completedVerification.Status, TaskVerificationStatusCompleted)
	}
	if completedVerification.Verdict != VerificationVerdictAccepted {
		t.Fatalf("verification verdict = %q, want %q", completedVerification.Verdict, VerificationVerdictAccepted)
	}
	if completedVerification.FinishedAt.IsZero() {
		t.Fatal("verification finished_at = zero, want non-zero")
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusCompleted {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusCompleted)
	}
	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(completed) error = %v", err)
	}
	if taskPage.Items[0].Status != TaskStatusCompleted {
		t.Fatalf("root task final status = %q, want %q", taskPage.Items[0].Status, TaskStatusCompleted)
	}
}

func TestIntegrationVerificationRejectFailsTaskAndRun(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "fail run when verification rejects output",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "root task produced invalid output",
		StructuredOutput: map[string]any{},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}
	completedVerification, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusFailed,
		Verdict:        VerificationVerdictRejected,
		Summary:        "structured_output is required",
		FailureClass:   "verification_rejected",
		TerminalReason: "structured_output is required",
	})
	if err != nil {
		t.Fatalf("CompleteVerification() error = %v", err)
	}
	if completedVerification.Status != TaskVerificationStatusFailed {
		t.Fatalf("verification status = %q, want %q", completedVerification.Status, TaskVerificationStatusFailed)
	}
	if completedVerification.Verdict != VerificationVerdictRejected {
		t.Fatalf("verification verdict = %q, want %q", completedVerification.Verdict, VerificationVerdictRejected)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusFailed {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusFailed)
	}
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if taskPage.Items[0].Status != TaskStatusFailed {
		t.Fatalf("root task status = %q, want %q", taskPage.Items[0].Status, TaskStatusFailed)
	}
}

func TestIntegrationKernelEndToEndRunBarrierResumeAndCompletion(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "kernel e2e should cover replan verification run barrier resume and completion",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask(root) error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask(root) = false, want true")
	}
	rootAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(root) error = %v", err)
	}
	runningRootAttempt, err := svc.StartAttempt(ctx, rootAttempt.ID, rootAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(root) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningRootAttempt.ID,
		ClaimToken:    runningRootAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "expand child tasks for kernel e2e",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{
					"id":             "verify-child",
					"goal":           "verify child",
					"worker_profile": "kernel-e2e.verify",
					"priority":       100,
				},
				map[string]any{
					"id":             "checkpoint-child",
					"goal":           "checkpoint child",
					"worker_profile": "kernel-e2e.checkpoint",
					"priority":       10,
				},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(root request_replan) error = %v", err)
	}
	drainRunPlanningIntents(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after replan) error = %v", err)
	}
	var verifyChild Task
	var checkpointChild Task
	for _, task := range taskPage.Items {
		switch task.Goal {
		case "verify child":
			verifyChild = task
		case "checkpoint child":
			checkpointChild = task
		}
	}
	if verifyChild.ID == "" || checkpointChild.ID == "" {
		t.Fatalf("child tasks not found after replan: %+v", taskPage.Items)
	}
	if verifyChild.Status != TaskStatusReady {
		t.Fatalf("verify child status = %q, want %q", verifyChild.Status, TaskStatusReady)
	}
	if checkpointChild.Status != TaskStatusReady {
		t.Fatalf("checkpoint child status = %q, want %q", checkpointChild.Status, TaskStatusReady)
	}

	setIntegrationTaskVerificationPolicy(t, ctx, pool, verifyChild.ID, map[string]any{
		"require_structured_output": true,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask(verify child) error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask(verify child) = false, want true")
	}
	verifyAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{"kernel-e2e.verify"},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(verify child) error = %v", err)
	}
	if verifyAttempt.TaskID != verifyChild.ID {
		t.Fatalf("verify child attempt task_id = %q, want %q", verifyAttempt.TaskID, verifyChild.ID)
	}
	runningVerifyAttempt, err := svc.StartAttempt(ctx, verifyAttempt.ID, verifyAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(verify child) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningVerifyAttempt.ID,
		ClaimToken:       runningVerifyAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "verify child produced output",
		StructuredOutput: map[string]any{"summary": "verify child done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt(verify child) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	verificationPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after verify child finalize) error = %v", err)
	}
	for _, task := range verificationPage.Items {
		if task.ID == verifyChild.ID && task.Status != TaskStatusVerifying {
			t.Fatalf("verify child status after finalize = %q, want %q", task.Status, TaskStatusVerifying)
		}
	}

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	if verification.TaskID != verifyChild.ID {
		t.Fatalf("claimed verification task_id = %q, want %q", verification.TaskID, verifyChild.ID)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         checkpointChild.ID,
		Question:       "pause sibling verification before continuing?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "resume", Kind: CheckpointOptionKindChoice, Label: "Resume"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(run barrier) error = %v", err)
	}

	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after barrier open) error = %v", err)
	}
	var pausedVerifyChild Task
	var waitingCheckpointChild Task
	for _, task := range taskPage.Items {
		switch task.ID {
		case verifyChild.ID:
			pausedVerifyChild = task
		case checkpointChild.ID:
			waitingCheckpointChild = task
		}
	}
	if pausedVerifyChild.Status != TaskStatusWaitingHuman || pausedVerifyChild.WaitingScope != "run" {
		t.Fatalf("paused verify child = %+v, want waiting_human(run)", pausedVerifyChild)
	}
	if waitingCheckpointChild.Status != TaskStatusWaitingHuman || waitingCheckpointChild.WaitingScope != "task" {
		t.Fatalf("checkpoint child = %+v, want waiting_human(task)", waitingCheckpointChild)
	}
	if waitingCheckpointChild.WaitingCheckpointID != checkpointResult.Checkpoint.ID {
		t.Fatalf("checkpoint child waiting_checkpoint_id = %q, want %q", waitingCheckpointChild.WaitingCheckpointID, checkpointResult.Checkpoint.ID)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(after barrier open) error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusWaitingHuman {
		t.Fatalf("run lifecycle_status after barrier open = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusWaitingHuman)
	}

	if _, err := svc.ResolveCheckpoint(ctx, caller, checkpointResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "resume",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after barrier resolve) error = %v", err)
	}
	var resumedVerifyChild Task
	var resumedCheckpointChild Task
	for _, task := range taskPage.Items {
		switch task.ID {
		case verifyChild.ID:
			resumedVerifyChild = task
		case checkpointChild.ID:
			resumedCheckpointChild = task
		}
	}
	if resumedVerifyChild.Status != TaskStatusVerifying {
		t.Fatalf("verify child status after barrier resolve = %q, want %q", resumedVerifyChild.Status, TaskStatusVerifying)
	}
	if resumedCheckpointChild.Status != TaskStatusReady {
		t.Fatalf("checkpoint child status after barrier resolve = %q, want %q", resumedCheckpointChild.Status, TaskStatusReady)
	}

	reclaimedVerification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification(after barrier resolve) error = %v", err)
	}
	if reclaimedVerification.ID != runningVerification.ID {
		t.Fatalf("reclaimed verification id = %q, want %q", reclaimedVerification.ID, runningVerification.ID)
	}
	restartedVerification, err := svc.StartVerification(ctx, reclaimedVerification.ID, reclaimedVerification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification(after barrier resolve) error = %v", err)
	}
	if _, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: restartedVerification.ID,
		ClaimToken:     restartedVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "verification resumed and accepted",
	}); err != nil {
		t.Fatalf("CompleteVerification(after barrier resolve) error = %v", err)
	}

	checkpointAttempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{"kernel-e2e.checkpoint"},
		LeaseTTLSeconds: 30,
	}, 3)
	if checkpointAttempt.TaskID != checkpointChild.ID {
		t.Fatalf("checkpoint child attempt task_id = %q, want %q", checkpointAttempt.TaskID, checkpointChild.ID)
	}
	runningCheckpointAttempt, err := svc.StartAttempt(ctx, checkpointAttempt.ID, checkpointAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(checkpoint child) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningCheckpointAttempt.ID,
		ClaimToken:       runningCheckpointAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "checkpoint child completed after resume",
		StructuredOutput: map[string]any{"summary": "checkpoint child done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt(checkpoint child) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	finalSnapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(final) error = %v", err)
	}
	if finalSnapshot.Run.LifecycleStatus != LifecycleStatusCompleted {
		t.Fatalf("final run lifecycle_status = %q, want %q", finalSnapshot.Run.LifecycleStatus, LifecycleStatusCompleted)
	}

	finalTaskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(final) error = %v", err)
	}
	completedTasks := 0
	for _, task := range finalTaskPage.Items {
		if task.Status == TaskStatusCompleted {
			completedTasks++
		}
	}
	if completedTasks != len(finalTaskPage.Items) {
		t.Fatalf("completed task count = %d, want %d; tasks = %+v", completedTasks, len(finalTaskPage.Items), finalTaskPage.Items)
	}
}

func TestIntegrationVerificationRejectRequestReplanEnqueuesReplan(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "replan after verification rejection",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"required_artifact_kinds": []any{"report"},
		"on_reject":               VerificationRejectActionReplan,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:  runningAttempt.ID,
		ClaimToken: runningAttempt.ClaimToken,
		Status:     TaskAttemptStatusCompleted,
		Summary:    "root task needs replan",
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{
					"goal":           "replacement child",
					"worker_profile": DefaultRootWorkerProfile,
				},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}
	completedVerification, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictRejected,
		Summary:        "artifact kind report is required",
		FailureClass:   "verification_requested_replan",
		TerminalReason: "artifact kind report is required",
		RequestReplan:  true,
	})
	if err != nil {
		t.Fatalf("CompleteVerification() error = %v", err)
	}
	if completedVerification.Status != TaskVerificationStatusCompleted {
		t.Fatalf("verification status = %q, want %q", completedVerification.Status, TaskVerificationStatusCompleted)
	}
	if completedVerification.Verdict != VerificationVerdictRejected {
		t.Fatalf("verification verdict = %q, want %q", completedVerification.Verdict, VerificationVerdictRejected)
	}

	intermediateSnapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(intermediate) error = %v", err)
	}
	if intermediateSnapshot.Run.PlanningStatus != PlanningStatusActive {
		t.Fatalf("intermediate planning_status = %q, want %q", intermediateSnapshot.Run.PlanningStatus, PlanningStatusActive)
	}
	processRunPlanningIntent(t, ctx, svc)

	rootTaskUUID, err := db.ParseUUID(handle.RootTaskID)
	if err != nil {
		t.Fatalf("parse root task uuid: %v", err)
	}
	rootTaskRow, err := svc.queries.GetOrchestrationTaskByID(ctx, rootTaskUUID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(root) error = %v", err)
	}
	if !rootTaskRow.SupersededByPlannerEpoch.Valid {
		t.Fatal("root task superseded_by_planner_epoch = null, want non-null")
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	replacementReady := false
	replacementTaskID := ""
	for _, task := range taskPage.Items {
		if task.ID == handle.RootTaskID {
			continue
		}
		if task.Goal == "replacement child" && task.Status == TaskStatusReady {
			replacementReady = true
			replacementTaskID = task.ID
		}
	}
	if !replacementReady {
		t.Fatal("replacement child task not found in ready state after verifier-triggered replan")
	}
	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(final) error = %v", err)
	}
	if snapshot.Run.PlanningStatus != PlanningStatusIdle {
		t.Fatalf("final planning_status = %q, want %q", snapshot.Run.PlanningStatus, PlanningStatusIdle)
	}

	verificationRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	verificationRows, err := svc.queries.ListCurrentOrchestrationTaskVerificationsByRun(ctx, verificationRunID)
	if err != nil {
		t.Fatalf("ListCurrentOrchestrationTaskVerificationsByRun() error = %v", err)
	}
	if len(verificationRows) != 1 {
		t.Fatalf("verification row count = %d, want 1", len(verificationRows))
	}
	if verificationRows[0].Status != TaskVerificationStatusCompleted {
		t.Fatalf("verification row status = %q, want %q", verificationRows[0].Status, TaskVerificationStatusCompleted)
	}

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask(replacement) error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask(replacement) = false, want true")
	}
	replacementAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(replacement) error = %v", err)
	}
	if replacementAttempt.TaskID != replacementTaskID {
		t.Fatalf("replacement attempt task_id = %q, want %q", replacementAttempt.TaskID, replacementTaskID)
	}
	runningReplacementAttempt, err := svc.StartAttempt(ctx, replacementAttempt.ID, replacementAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(replacement) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningReplacementAttempt.ID,
		ClaimToken:       runningReplacementAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "replacement child done",
		StructuredOutput: map[string]any{"summary": "replacement done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt(replacement) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	snapshot, err = svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(after replacement) error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusCompleted {
		t.Fatalf("run lifecycle_status after replacement = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusCompleted)
	}
}

func TestIntegrationProcessNextVerificationExecutesCreatedVerification(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "server verifier loop should process created verification",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "root task produced structured output",
		StructuredOutput: map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(before verification) error = %v", err)
	}
	if len(taskPage.Items) != 1 {
		t.Fatalf("ListRunTasks(before verification) len = %d, want 1", len(taskPage.Items))
	}
	if taskPage.Items[0].Status != TaskStatusVerifying {
		t.Fatalf("root task status before verification = %q, want %q", taskPage.Items[0].Status, TaskStatusVerifying)
	}

	runUUID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	verificationRows, err := svc.queries.ListCurrentOrchestrationTaskVerificationsByRun(ctx, runUUID)
	if err != nil {
		t.Fatalf("ListCurrentOrchestrationTaskVerificationsByRun(before) error = %v", err)
	}
	if len(verificationRows) != 1 {
		t.Fatalf("verification row count before processing = %d, want 1", len(verificationRows))
	}
	if verificationRows[0].Status != TaskVerificationStatusCreated {
		t.Fatalf("verification row status before processing = %q, want %q", verificationRows[0].Status, TaskVerificationStatusCreated)
	}
	if int(verificationRows[0].AttemptNo) != runningAttempt.AttemptNo {
		t.Fatalf("verification row attempt_no before processing = %d, want %d", verificationRows[0].AttemptNo, runningAttempt.AttemptNo)
	}

	processed, err := svc.ProcessNextVerification(ctx, "server-verifier-"+uuid.NewString(), "lease-"+uuid.NewString(), []string{DefaultVerifierProfile}, 30)
	if err != nil {
		t.Fatalf("ProcessNextVerification() error = %v", err)
	}
	if !processed {
		t.Fatal("ProcessNextVerification() = false, want true")
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusCompleted {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusCompleted)
	}

	verificationRows, err = svc.queries.ListCurrentOrchestrationTaskVerificationsByRun(ctx, runUUID)
	if err != nil {
		t.Fatalf("ListCurrentOrchestrationTaskVerificationsByRun(after) error = %v", err)
	}
	if len(verificationRows) != 1 {
		t.Fatalf("verification row count after processing = %d, want 1", len(verificationRows))
	}
	if verificationRows[0].Status != TaskVerificationStatusCompleted {
		t.Fatalf("verification row status after processing = %q, want %q", verificationRows[0].Status, TaskVerificationStatusCompleted)
	}
	if verificationRows[0].Verdict != VerificationVerdictAccepted {
		t.Fatalf("verification row verdict after processing = %q, want %q", verificationRows[0].Verdict, VerificationVerdictAccepted)
	}
}

func TestIntegrationVerificationAcceptedRequestReplanEnqueuesReplan(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "verified request_replan should still enqueue replacement plan",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningAttempt.ID,
		ClaimToken:    runningAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "root task completed with replacement plan",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"summary": "done",
			"child_tasks": []any{
				map[string]any{
					"goal":           "replacement child",
					"worker_profile": DefaultRootWorkerProfile,
				},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}
	if _, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "verified",
	}); err != nil {
		t.Fatalf("CompleteVerification() error = %v", err)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(after verification) error = %v", err)
	}
	if snapshot.Run.PlanningStatus != PlanningStatusActive {
		t.Fatalf("planning_status after accepted request_replan = %q, want %q", snapshot.Run.PlanningStatus, PlanningStatusActive)
	}
	processRunPlanningIntent(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	var replacementReady bool
	for _, task := range taskPage.Items {
		if task.ID == handle.RootTaskID {
			if task.SupersededByPlannerEpoch == 0 {
				t.Fatal("root task superseded_by_planner_epoch = 0, want non-zero")
			}
			continue
		}
		if task.Status == TaskStatusReady {
			replacementReady = true
		}
	}
	if !replacementReady {
		t.Fatal("replacement child task not found in ready state after accepted request_replan")
	}
}

func TestIntegrationCompleteVerificationRejectsUnauthorizedRequestReplan(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "verification policy should fence request_replan",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"required_artifact_kinds": []any{"report"},
		"on_reject":               VerificationRejectActionFailTask,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:  runningAttempt.ID,
		ClaimToken: runningAttempt.ClaimToken,
		Status:     TaskAttemptStatusCompleted,
		Summary:    "root task needs report artifact",
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{
					"goal":           "replacement child",
					"worker_profile": DefaultRootWorkerProfile,
				},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}
	if _, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictRejected,
		Summary:        "artifact kind report is required",
		FailureClass:   "verification_requested_replan",
		TerminalReason: "artifact kind report is required",
		RequestReplan:  true,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CompleteVerification(unauthorized request_replan) error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestIntegrationCompleteVerificationRejectsAcceptedFailedCombination(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "reject contradictory verification completion",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "root task produced structured output",
		StructuredOutput: map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}
	if _, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusFailed,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "contradictory",
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CompleteVerification(contradictory) error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestIntegrationVerificationLeaseExpiryRecoveryFailsRunOnce(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "recover expired verification lease exactly once",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	if dispatched, err := svc.DispatchNextReadyTask(ctx); err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	} else if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "root task produced structured output",
		StructuredOutput: map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}

	pgVerificationID, err := db.ParseUUID(runningVerification.ID)
	if err != nil {
		t.Fatalf("parse verification uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_verifications SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", pgVerificationID); err != nil {
		t.Fatalf("expire verification lease: %v", err)
	}

	if _, err := svc.HeartbeatVerification(ctx, VerificationHeartbeat{
		VerificationID:  runningVerification.ID,
		ClaimToken:      runningVerification.ClaimToken,
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrVerificationLeaseConflict) {
		t.Fatalf("HeartbeatVerification(expired lease) error = %v, want %v", err, ErrVerificationLeaseConflict)
	}
	if _, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "verified",
	}); !errors.Is(err, ErrVerificationLeaseConflict) {
		t.Fatalf("CompleteVerification(expired lease) error = %v, want %v", err, ErrVerificationLeaseConflict)
	}

	recovered, err := svc.RecoverExpiredVerifications(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredVerifications(first) error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverExpiredVerifications(first) = %d, want 1", recovered)
	}
	recovered, err = svc.RecoverExpiredVerifications(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredVerifications(second) error = %v", err)
	}
	if recovered != 0 {
		t.Fatalf("RecoverExpiredVerifications(second) = %d, want 0", recovered)
	}

	verificationRow, err := svc.queries.GetOrchestrationTaskVerificationByID(ctx, pgVerificationID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskVerificationByID() error = %v", err)
	}
	if verificationRow.Status != TaskVerificationStatusLost {
		t.Fatalf("verification row status = %q, want %q", verificationRow.Status, TaskVerificationStatusLost)
	}
	if verificationRow.TerminalReason != "verification lease expired" {
		t.Fatalf("verification row terminal_reason = %q, want %q", verificationRow.TerminalReason, "verification lease expired")
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusFailed {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusFailed)
	}
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if taskPage.Items[0].Status != TaskStatusFailed {
		t.Fatalf("root task status = %q, want %q", taskPage.Items[0].Status, TaskStatusFailed)
	}
}

func TestIntegrationInvalidRequestReplanFailsIntentWithoutStrandingRun(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "invalid request_replan should not strand run",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningAttempt.ID,
		ClaimToken:    runningAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "invalid replan output",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{"id": "a", "goal": "child a", "depends_on": []any{"missing"}},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(invalid request_replan) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusCompleted {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusCompleted)
	}
	if snapshot.Run.PlanningStatus != PlanningStatusIdle {
		t.Fatalf("run planning_status = %q, want %q", snapshot.Run.PlanningStatus, PlanningStatusIdle)
	}

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	intentRows, err := svc.queries.ListCurrentOrchestrationTaskAttemptsByRun(ctx, pgRunID)
	if err != nil {
		t.Fatalf("ListCurrentOrchestrationTaskAttemptsByRun() error = %v", err)
	}
	if len(intentRows) != 1 || intentRows[0].Status != TaskAttemptStatusCompleted {
		t.Fatalf("attempt rows = %+v, want single completed attempt", intentRows)
	}

	var failedIntentCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM orchestration_planning_intents
		WHERE run_id = $1
		  AND kind = $2
		  AND status = $3
	`, pgRunID, PlanningIntentKindAttemptFinalize, PlanningIntentStatusFailed).Scan(&failedIntentCount); err != nil {
		t.Fatalf("query failed attempt_finalize intents: %v", err)
	}
	if failedIntentCount != 1 {
		t.Fatalf("failed attempt_finalize intent count = %d, want 1", failedIntentCount)
	}
}

func TestIntegrationReplanSupersedesOldSubtreeAndOpenCheckpoint(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "supersede old subtree and checkpoint during replan",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(root) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(root) = false, want true")
	}
	rootAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(root) error = %v", err)
	}
	runningRootAttempt, err := svc.StartAttempt(ctx, rootAttempt.ID, rootAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(root) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningRootAttempt.ID,
		ClaimToken:    runningRootAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "expand first subtree",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{"id": "old", "goal": "old child"},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(root request_replan) error = %v", err)
	}
	drainRunPlanningIntents(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after first replan) error = %v", err)
	}
	var oldChild Task
	for _, task := range taskPage.Items {
		if task.Goal == "old child" {
			oldChild = task
			break
		}
	}
	if oldChild.ID == "" {
		t.Fatal("old child task not found after first replan")
	}

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         oldChild.ID,
		Question:       "approve old child?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "continue", Kind: CheckpointOptionKindChoice, Label: "Continue"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(old child) error = %v", err)
	}

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	pgSourceTaskID, err := db.ParseUUID(oldChild.ID)
	if err != nil {
		t.Fatalf("parse source task uuid: %v", err)
	}
	runRow, err := svc.queries.GetOrchestrationRunByID(ctx, pgRunID)
	if err != nil {
		t.Fatalf("GetOrchestrationRunByID() error = %v", err)
	}
	_, replanIntentUUID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(replan intent) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               replanIntentUUID,
		RunID:            pgRunID,
		TaskID:           pgSourceTaskID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindReplan,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: runRow.PlannerEpoch,
		Payload: marshalJSON(map[string]any{
			"run_id":         handle.RunID,
			"source_task_id": oldChild.ID,
			"reason":         "integration_test.manual_replan",
			"replacement_plan": map[string]any{
				"child_tasks": []any{
					map[string]any{"id": "replacement", "goal": "replacement child"},
				},
			},
		}),
	}); err != nil {
		t.Fatalf("CreateOrchestrationPlanningIntent(replan) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after supersede replan) error = %v", err)
	}
	var replacementChild Task
	var refreshedOldChild Task
	for _, task := range taskPage.Items {
		switch task.Goal {
		case "old child":
			refreshedOldChild = task
		case "replacement child":
			replacementChild = task
		}
	}
	if replacementChild.ID == "" {
		t.Fatal("replacement child task not found after replan")
	}
	if replacementChild.Status != TaskStatusReady {
		t.Fatalf("replacement child status = %q, want %q", replacementChild.Status, TaskStatusReady)
	}
	if refreshedOldChild.ID == "" {
		t.Fatal("old child task missing after supersede")
	}
	if refreshedOldChild.SupersededByPlannerEpoch == 0 {
		t.Fatalf("old child superseded_by_planner_epoch = 0, want non-zero")
	}
	if refreshedOldChild.WaitingCheckpointID != "" {
		t.Fatalf("old child waiting_checkpoint_id = %q, want empty after supersede", refreshedOldChild.WaitingCheckpointID)
	}
	if refreshedOldChild.WaitingScope != "" {
		t.Fatalf("old child waiting_scope = %q, want empty after supersede", refreshedOldChild.WaitingScope)
	}

	checkpointPage, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	var refreshedCheckpoint HumanCheckpoint
	for _, checkpoint := range checkpointPage.Items {
		if checkpoint.ID == checkpointResult.Checkpoint.ID {
			refreshedCheckpoint = checkpoint
			break
		}
	}
	if refreshedCheckpoint.ID == "" {
		t.Fatal("old checkpoint missing after supersede")
	}
	if refreshedCheckpoint.Status != CheckpointStatusSuperseded {
		t.Fatalf("old checkpoint status = %q, want %q", refreshedCheckpoint.Status, CheckpointStatusSuperseded)
	}
	if refreshedCheckpoint.SupersededByPlannerEpoch == 0 {
		t.Fatalf("old checkpoint superseded_by_planner_epoch = 0, want non-zero")
	}

	rootFound := false
	for _, task := range taskPage.Items {
		if task.ID != handle.RootTaskID {
			continue
		}
		rootFound = true
		if task.SupersededByPlannerEpoch == 0 {
			t.Fatal("root task superseded_by_planner_epoch = 0, want non-zero")
		}
		if task.Status != TaskStatusCompleted {
			t.Fatalf("root task status = %q, want %q", task.Status, TaskStatusCompleted)
		}
	}
	if !rootFound {
		t.Fatal("root task missing after subtree replan")
	}

	if _, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         refreshedOldChild.ID,
		Question:       "obsolete task should not accept checkpoint?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "noop", Kind: CheckpointOptionKindChoice, Label: "Noop"},
		},
	}); !errors.Is(err, ErrTaskImmutable) {
		t.Fatalf("CreateHumanCheckpoint(superseded child) error = %v, want %v", err, ErrTaskImmutable)
	}

	barrierResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         replacementChild.ID,
		Question:       "replacement child barrier should ignore superseded waiters?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "continue", Kind: CheckpointOptionKindChoice, Label: "Continue"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(replacement barrier) error = %v", err)
	}
	if barrierResult.Checkpoint.ID == "" {
		t.Fatal("replacement barrier checkpoint id = empty, want non-empty")
	}
	if _, err := svc.ResolveCheckpoint(ctx, caller, barrierResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "continue",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("ResolveCheckpoint(replacement barrier) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	dispatched, err = svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(replacement) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(replacement) = false, want true")
	}
	replacementAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(replacement) error = %v", err)
	}
	if replacementAttempt.TaskID != replacementChild.ID {
		t.Fatalf("claimed task_id = %q, want replacement child %q", replacementAttempt.TaskID, replacementChild.ID)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.PlanningStatus != PlanningStatusIdle {
		t.Fatalf("run planning_status = %q, want %q", snapshot.Run.PlanningStatus, PlanningStatusIdle)
	}
}

func TestIntegrationReplanRejectsActiveSubtree(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "reject replans against active subtree",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(root) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(root) = false, want true")
	}
	rootAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(root) error = %v", err)
	}
	runningRootAttempt, err := svc.StartAttempt(ctx, rootAttempt.ID, rootAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(root) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningRootAttempt.ID,
		ClaimToken:    runningRootAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "create child subtree",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{"id": "active", "goal": "active child"},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(root request_replan) error = %v", err)
	}
	drainRunPlanningIntents(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after first replan) error = %v", err)
	}
	var activeChild Task
	for _, task := range taskPage.Items {
		if task.Goal == "active child" {
			activeChild = task
			break
		}
	}
	if activeChild.ID == "" {
		t.Fatal("active child task not found")
	}

	dispatched, err = svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(active child) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(active child) = false, want true")
	}
	activeAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(active child) error = %v", err)
	}
	runningActiveAttempt, err := svc.StartAttempt(ctx, activeAttempt.ID, activeAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(active child) error = %v", err)
	}

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	pgSourceTaskID, err := db.ParseUUID(activeChild.ID)
	if err != nil {
		t.Fatalf("parse source task uuid: %v", err)
	}
	runRow, err := svc.queries.GetOrchestrationRunByID(ctx, pgRunID)
	if err != nil {
		t.Fatalf("GetOrchestrationRunByID() error = %v", err)
	}
	_, replanIntentUUID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(replan intent) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               replanIntentUUID,
		RunID:            pgRunID,
		TaskID:           pgSourceTaskID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindReplan,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: runRow.PlannerEpoch,
		Payload: marshalJSON(map[string]any{
			"run_id":         handle.RunID,
			"source_task_id": activeChild.ID,
			"attempt_id":     runningActiveAttempt.ID,
			"reason":         "integration_test.active_subtree_replan",
			"replacement_plan": map[string]any{
				"child_tasks": []any{
					map[string]any{"id": "replacement", "goal": "replacement"},
				},
			},
		}),
	}); err != nil {
		t.Fatalf("CreateOrchestrationPlanningIntent(replan) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	intentRow, err := svc.queries.GetOrchestrationPlanningIntentByID(ctx, replanIntentUUID)
	if err != nil {
		t.Fatalf("GetOrchestrationPlanningIntentByID() error = %v", err)
	}
	if intentRow.Status != PlanningIntentStatusFailed {
		t.Fatalf("replan intent status = %q, want %q", intentRow.Status, PlanningIntentStatusFailed)
	}

	refreshedTaskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after failed replan) error = %v", err)
	}
	for _, task := range refreshedTaskPage.Items {
		if task.ID != activeChild.ID {
			continue
		}
		if task.SupersededByPlannerEpoch != 0 {
			t.Fatalf("active child superseded_by_planner_epoch = %d, want 0", task.SupersededByPlannerEpoch)
		}
		if task.Status != TaskStatusRunning {
			t.Fatalf("active child status = %q, want %q", task.Status, TaskStatusRunning)
		}
	}
}

func TestIntegrationSupersededRunningAttemptIsRejectedAndRecoveredWithoutFailingRun(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "fence superseded running attempts",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}

	pgTaskID, err := db.ParseUUID(handle.RootTaskID)
	if err != nil {
		t.Fatalf("parse task uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_tasks SET superseded_by_planner_epoch = 99, updated_at = now() WHERE id = $1", pgTaskID); err != nil {
		t.Fatalf("mark task superseded: %v", err)
	}

	if _, err := svc.HeartbeatAttempt(ctx, AttemptHeartbeat{
		AttemptID:       runningAttempt.ID,
		ClaimToken:      runningAttempt.ClaimToken,
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrAttemptImmutable) {
		t.Fatalf("HeartbeatAttempt(superseded running task) error = %v, want %v", err, ErrAttemptImmutable)
	}

	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:  runningAttempt.ID,
		ClaimToken: runningAttempt.ClaimToken,
		Status:     TaskAttemptStatusCompleted,
		Summary:    "stale result",
	}); !errors.Is(err, ErrAttemptImmutable) {
		t.Fatalf("CompleteAttempt(superseded running task) error = %v, want %v", err, ErrAttemptImmutable)
	}

	pgAttemptID, err := db.ParseUUID(runningAttempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("expire attempt lease: %v", err)
	}

	recovered, err := svc.RecoverExpiredAttempts(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredAttempts() error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverExpiredAttempts() = %d, want 1", recovered)
	}

	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusLost {
		t.Fatalf("attempt status after recovery = %q, want %q", attemptRow.Status, TaskAttemptStatusLost)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus == LifecycleStatusFailed {
		t.Fatalf("run lifecycle_status = %q, want non-failed", snapshot.Run.LifecycleStatus)
	}
}

func TestIntegrationDependentChildTaskWaitsForCompletedPredecessor(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "respect child task dependencies",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(root) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(root) = false, want true")
	}
	rootAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(root) error = %v", err)
	}
	runningRootAttempt, err := svc.StartAttempt(ctx, rootAttempt.ID, rootAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(root) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningRootAttempt.ID,
		ClaimToken:    runningRootAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "expand dependent child tasks",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{
					"id":   "first",
					"goal": "child task one",
				},
				map[string]any{
					"id":         "second",
					"goal":       "child task two",
					"depends_on": []any{"first"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(root request_replan) error = %v", err)
	}
	drainRunPlanningIntents(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after replan) error = %v", err)
	}
	var firstChild, secondChild Task
	for _, task := range taskPage.Items {
		if task.ID == handle.RootTaskID {
			continue
		}
		switch task.Goal {
		case "child task one":
			firstChild = task
		case "child task two":
			secondChild = task
		}
	}
	if firstChild.ID == "" || secondChild.ID == "" {
		t.Fatalf("child tasks not found: %+v", taskPage.Items)
	}
	if firstChild.Status != TaskStatusReady {
		t.Fatalf("first child status = %q, want %q", firstChild.Status, TaskStatusReady)
	}
	if secondChild.Status != TaskStatusCreated {
		t.Fatalf("second child status = %q, want %q", secondChild.Status, TaskStatusCreated)
	}

	dispatched, err = svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(first child) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(first child) = false, want true")
	}
	firstChildAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(first child) error = %v", err)
	}
	if firstChildAttempt.TaskID != firstChild.ID {
		t.Fatalf("first claimed child task_id = %q, want %q", firstChildAttempt.TaskID, firstChild.ID)
	}
	runningChildAttempt, err := svc.StartAttempt(ctx, firstChildAttempt.ID, firstChildAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(first child) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:  runningChildAttempt.ID,
		ClaimToken: runningChildAttempt.ClaimToken,
		Status:     TaskAttemptStatusCompleted,
		Summary:    "complete first child",
	}); err != nil {
		t.Fatalf("CompleteAttempt(first child) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	dispatched, err = svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(dependent child) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(dependent child) = false, want true")
	}
	dependentAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(dependent child) error = %v", err)
	}
	if dependentAttempt.TaskID != secondChild.ID {
		t.Fatalf("dependent claimed task_id = %q, want %q", dependentAttempt.TaskID, secondChild.ID)
	}
}

func TestIntegrationFailedDependencyBlocksSuccessorTask(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "block dependent tasks after predecessor failure",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(root) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(root) = false, want true")
	}
	rootAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(root) error = %v", err)
	}
	runningRootAttempt, err := svc.StartAttempt(ctx, rootAttempt.ID, rootAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(root) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningRootAttempt.ID,
		ClaimToken:    runningRootAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "expand failure propagation tasks",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{"id": "first", "goal": "child task one"},
				map[string]any{"id": "second", "goal": "child task two", "depends_on": []any{"first"}},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(root request_replan) error = %v", err)
	}
	drainRunPlanningIntents(t, ctx, svc)

	dispatched, err = svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(first child) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(first child) = false, want true")
	}
	firstChildAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(first child) error = %v", err)
	}
	runningChildAttempt, err := svc.StartAttempt(ctx, firstChildAttempt.ID, firstChildAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(first child) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:      runningChildAttempt.ID,
		ClaimToken:     runningChildAttempt.ClaimToken,
		Status:         TaskAttemptStatusFailed,
		FailureClass:   "integration_failure",
		TerminalReason: "first child failed",
		Summary:        "first child failed",
	}); err != nil {
		t.Fatalf("CompleteAttempt(first child failed) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	var blockedFound bool
	for _, task := range taskPage.Items {
		if task.Goal != "child task two" {
			continue
		}
		if task.Status != TaskStatusBlocked {
			t.Fatalf("dependent task status = %q, want %q", task.Status, TaskStatusBlocked)
		}
		if !strings.Contains(task.BlockedReason, "dependency_failed:") {
			t.Fatalf("dependent task blocked_reason = %q, want dependency_failed:*", task.BlockedReason)
		}
		blockedFound = true
	}
	if !blockedFound {
		t.Fatal("dependent blocked task not found")
	}
}

func TestIntegrationJoinTaskAutoCompletesAfterPredecessors(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "auto-complete join tasks",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(root) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(root) = false, want true")
	}
	rootAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(root) error = %v", err)
	}
	runningRootAttempt, err := svc.StartAttempt(ctx, rootAttempt.ID, rootAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(root) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:     runningRootAttempt.ID,
		ClaimToken:    runningRootAttempt.ClaimToken,
		Status:        TaskAttemptStatusCompleted,
		Summary:       "expand join tasks",
		RequestReplan: true,
		StructuredOutput: map[string]any{
			"child_tasks": []any{
				map[string]any{"id": "a", "goal": "branch a"},
				map[string]any{"id": "b", "goal": "branch b"},
				map[string]any{"id": "j", "kind": "join", "goal": "join branches", "depends_on": []any{"a", "b"}},
			},
		},
	}); err != nil {
		t.Fatalf("CompleteAttempt(root request_replan) error = %v", err)
	}
	drainRunPlanningIntents(t, ctx, svc)

	for i := 0; i < 2; i++ {
		dispatched, err = svc.DispatchNextReadyTask(ctx)
		if err != nil {
			t.Fatalf("DispatchNextReadyTask(branch %d) error = %v", i+1, err)
		}
		if !dispatched {
			t.Fatalf("DispatchNextReadyTask(branch %d) = false, want true", i+1)
		}
		attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
			WorkerID:        "worker-" + uuid.NewString(),
			ExecutorID:      DefaultWorkerExecutorID,
			WorkerProfiles:  []string{DefaultRootWorkerProfile},
			LeaseTTLSeconds: 30,
		})
		if err != nil {
			t.Fatalf("ClaimNextAttempt(branch %d) error = %v", i+1, err)
		}
		runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
		if err != nil {
			t.Fatalf("StartAttempt(branch %d) error = %v", i+1, err)
		}
		if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
			AttemptID:  runningAttempt.ID,
			ClaimToken: runningAttempt.ClaimToken,
			Status:     TaskAttemptStatusCompleted,
			Summary:    "branch completed",
		}); err != nil {
			t.Fatalf("CompleteAttempt(branch %d) error = %v", i+1, err)
		}
		processRunPlanningIntent(t, ctx, svc)
	}

	dispatched, err = svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask(join activation) error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask(join activation) = false, want true")
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusCompleted {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusCompleted)
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	var joinFound bool
	for _, task := range taskPage.Items {
		if task.Kind != "join" {
			continue
		}
		if task.Status != TaskStatusCompleted {
			t.Fatalf("join task status = %q, want %q", task.Status, TaskStatusCompleted)
		}
		joinFound = true
	}
	if !joinFound {
		t.Fatal("join task not found")
	}
}

func TestIntegrationStartRunCanonicalizesIdempotencyKey(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}
	idem := "start-" + uuid.NewString()

	handle1, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "idempotent start run",
		IdempotencyKey: idem + " ",
	})
	if err != nil {
		t.Fatalf("StartRun(first) error = %v", err)
	}
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle1.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	handle2, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "idempotent start run",
		IdempotencyKey: idem,
	})
	if err != nil {
		t.Fatalf("StartRun(second) error = %v", err)
	}
	if handle2.RunID != handle1.RunID {
		t.Fatalf("StartRun(canonical retry) run_id = %q, want %q", handle2.RunID, handle1.RunID)
	}
}

func TestIntegrationCreateHumanCheckpointCanonicalizesIdempotencyKey(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}
	idem := "checkpoint-" + uuid.NewString()

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "idempotent checkpoint creation",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	first, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "ship it?",
		IdempotencyKey: idem + " ",
		Options: []CheckpointOption{
			{ID: "ship", Kind: CheckpointOptionKindChoice, Label: "Ship"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(first) error = %v", err)
	}

	second, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "ship it?",
		IdempotencyKey: idem,
		Options: []CheckpointOption{
			{ID: "ship", Kind: CheckpointOptionKindChoice, Label: "Ship"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(second) error = %v", err)
	}
	if second.Checkpoint.ID != first.Checkpoint.ID {
		t.Fatalf("CreateHumanCheckpoint(canonical retry) checkpoint_id = %q, want %q", second.Checkpoint.ID, first.Checkpoint.ID)
	}

	page, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("ListRunCheckpoints() len = %d, want 1", len(page.Items))
	}
}

func TestIntegrationCheckpointMutationResultsExposeStableSnapshotSeq(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "checkpoint writes should return stable replay bounds",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	createResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "continue?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "continue", Kind: CheckpointOptionKindChoice, Label: "Continue"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}
	if createResult.SnapshotSeq == 0 {
		t.Fatal("CreateHumanCheckpoint() snapshot_seq = 0, want committed seq")
	}

	resolveResult, err := svc.ResolveCheckpoint(ctx, caller, createResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "continue",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	if resolveResult.SnapshotSeq <= createResult.SnapshotSeq {
		t.Fatalf("ResolveCheckpoint() snapshot_seq = %d, want > %d", resolveResult.SnapshotSeq, createResult.SnapshotSeq)
	}

	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{
		Limit:    100,
		UntilSeq: createResult.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunEvents(until create snapshot) error = %v", err)
	}
	for _, event := range eventPage.Items {
		if event.Type == "run.event.hitl.resolved" {
			t.Fatalf("ListRunEvents(until create snapshot) unexpectedly included %q", event.Type)
		}
	}
}

func TestIntegrationCommitArtifactEmitsCommittedEventAndRefreshesProjection(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "artifact commits should update projections",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	artifactResult, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Kind:           "report",
		URI:            "memoh://artifact/report.md",
		Version:        "v1",
		Digest:         "sha256:abc123",
		ContentType:    "text/markdown",
		Summary:        "final report",
		Metadata:       map[string]any{"source": "integration-test"},
		IdempotencyKey: "artifact-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CommitArtifact() error = %v", err)
	}
	if artifactResult.SnapshotSeq == 0 {
		t.Fatal("CommitArtifact() snapshot_seq = 0, want committed seq")
	}

	artifactPage, err := svc.ListRunArtifacts(ctx, caller, handle.RunID, ListRunArtifactsRequest{
		TaskID:  strings.ToUpper(handle.RootTaskID),
		AsOfSeq: artifactResult.SnapshotSeq,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	if len(artifactPage.Items) != 1 {
		t.Fatalf("ListRunArtifacts() len = %d, want 1", len(artifactPage.Items))
	}
	if artifactPage.Items[0].ID != artifactResult.Artifact.ID {
		t.Fatalf("ListRunArtifacts() artifact_id = %q, want %q", artifactPage.Items[0].ID, artifactResult.Artifact.ID)
	}

	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{
		Limit:    50,
		UntilSeq: artifactResult.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	var found bool
	for _, event := range eventPage.Items {
		if event.Type != "run.event.artifact.committed" {
			continue
		}
		if event.Payload["artifact_id"] != artifactResult.Artifact.ID {
			t.Fatalf("artifact committed event artifact_id = %v, want %q", event.Payload["artifact_id"], artifactResult.Artifact.ID)
		}
		if event.Payload["run_id"] != handle.RunID {
			t.Fatalf("artifact committed event run_id = %v, want %q", event.Payload["run_id"], handle.RunID)
		}
		if event.Payload["task_id"] != handle.RootTaskID {
			t.Fatalf("artifact committed event task_id = %v, want %q", event.Payload["task_id"], handle.RootTaskID)
		}
		found = true
	}
	if !found {
		t.Fatal("ListRunEvents() missing run.event.artifact.committed")
	}
}

func TestIntegrationListRunTasksPaginationUsesStableSnapshotSeq(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "task pagination should be stable across pages",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	taskIDs := []string{
		handle.RootTaskID,
		createIntegrationChildTask(t, ctx, svc, handle.RunID, "child one", "run.child.one"),
		createIntegrationChildTask(t, ctx, svc, handle.RunID, "child two", "run.child.two"),
	}

	if _, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Kind:           "report",
		URI:            "memoh://artifact/task-pagination.md",
		Version:        "v1",
		Digest:         "sha256:task-pagination",
		ContentType:    "text/markdown",
		Summary:        "refresh task snapshot after direct child inserts",
		IdempotencyKey: "artifact-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("CommitArtifact(refresh task snapshot) error = %v", err)
	}

	page1, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 2})
	if err != nil {
		t.Fatalf("ListRunTasks(page1) error = %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("ListRunTasks(page1) len = %d, want 2", len(page1.Items))
	}
	if page1.SnapshotSeq == 0 || page1.NextAfter == "" {
		t.Fatalf("ListRunTasks(page1) snapshot_seq=%d next_after=%q, want both populated", page1.SnapshotSeq, page1.NextAfter)
	}

	page2, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 2, After: page1.NextAfter})
	if err != nil {
		t.Fatalf("ListRunTasks(page2) error = %v", err)
	}
	if page2.SnapshotSeq != page1.SnapshotSeq {
		t.Fatalf("ListRunTasks(page2) snapshot_seq = %d, want %d", page2.SnapshotSeq, page1.SnapshotSeq)
	}
	if page2.NextAfter != "" {
		t.Fatalf("ListRunTasks(page2) next_after = %q, want empty", page2.NextAfter)
	}

	seen := make(map[string]struct{}, len(taskIDs))
	for _, item := range append(append([]Task{}, page1.Items...), page2.Items...) {
		if _, ok := seen[item.ID]; ok {
			t.Fatalf("duplicate task %q across pages", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	if len(seen) != len(taskIDs) {
		t.Fatalf("seen task count = %d, want %d", len(seen), len(taskIDs))
	}
	for _, taskID := range taskIDs {
		if _, ok := seen[taskID]; !ok {
			t.Fatalf("missing task %q from paginated read", taskID)
		}
	}
}

func TestIntegrationListRunCheckpointsAndArtifactsSupportStableHistoricalPaging(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "checkpoint and artifact pagination should honor historical cuts",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	taskIDs := []string{
		handle.RootTaskID,
		createIntegrationChildTask(t, ctx, svc, handle.RunID, "child checkpoint one", "run.cp.one"),
		createIntegrationChildTask(t, ctx, svc, handle.RunID, "child checkpoint two", "run.cp.two"),
	}

	for idx, taskID := range taskIDs {
		if _, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
			RunID:          handle.RunID,
			TaskID:         taskID,
			Kind:           "report",
			URI:            "memoh://artifact/" + strconv.Itoa(idx) + ".md",
			Version:        "v1",
			Digest:         "sha256:artifact-" + strconv.Itoa(idx),
			ContentType:    "text/markdown",
			Summary:        "artifact " + strconv.Itoa(idx),
			IdempotencyKey: "artifact-" + uuid.NewString(),
		}); err != nil {
			t.Fatalf("CommitArtifact(%d) error = %v", idx, err)
		}
	}

	var checkpointIDs []string
	for idx, taskID := range taskIDs {
		result, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
			RunID:          handle.RunID,
			TaskID:         taskID,
			Question:       "checkpoint " + strconv.Itoa(idx) + "?",
			IdempotencyKey: "checkpoint-" + uuid.NewString(),
			Options: []CheckpointOption{
				{ID: "continue", Kind: CheckpointOptionKindChoice, Label: "Continue"},
			},
		})
		if err != nil {
			t.Fatalf("CreateHumanCheckpoint(%d) error = %v", idx, err)
		}
		checkpointIDs = append(checkpointIDs, result.Checkpoint.ID)
	}

	checkpointPage1, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 2})
	if err != nil {
		t.Fatalf("ListRunCheckpoints(page1) error = %v", err)
	}
	if len(checkpointPage1.Items) != 2 || checkpointPage1.NextAfter == "" || checkpointPage1.SnapshotSeq == 0 {
		t.Fatalf("ListRunCheckpoints(page1) items=%d next_after=%q snapshot_seq=%d", len(checkpointPage1.Items), checkpointPage1.NextAfter, checkpointPage1.SnapshotSeq)
	}
	checkpointPage2, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 2, After: checkpointPage1.NextAfter})
	if err != nil {
		t.Fatalf("ListRunCheckpoints(page2) error = %v", err)
	}
	if checkpointPage2.SnapshotSeq != checkpointPage1.SnapshotSeq {
		t.Fatalf("ListRunCheckpoints(page2) snapshot_seq = %d, want %d", checkpointPage2.SnapshotSeq, checkpointPage1.SnapshotSeq)
	}
	if checkpointPage2.NextAfter != "" {
		t.Fatalf("ListRunCheckpoints(page2) next_after = %q, want empty", checkpointPage2.NextAfter)
	}

	artifactPage1, err := svc.ListRunArtifacts(ctx, caller, handle.RunID, ListRunArtifactsRequest{Limit: 2})
	if err != nil {
		t.Fatalf("ListRunArtifacts(page1) error = %v", err)
	}
	if len(artifactPage1.Items) != 2 || artifactPage1.NextAfter == "" || artifactPage1.SnapshotSeq == 0 {
		t.Fatalf("ListRunArtifacts(page1) items=%d next_after=%q snapshot_seq=%d", len(artifactPage1.Items), artifactPage1.NextAfter, artifactPage1.SnapshotSeq)
	}
	artifactPage2, err := svc.ListRunArtifacts(ctx, caller, handle.RunID, ListRunArtifactsRequest{Limit: 2, After: artifactPage1.NextAfter})
	if err != nil {
		t.Fatalf("ListRunArtifacts(page2) error = %v", err)
	}
	if artifactPage2.SnapshotSeq != artifactPage1.SnapshotSeq {
		t.Fatalf("ListRunArtifacts(page2) snapshot_seq = %d, want %d", artifactPage2.SnapshotSeq, artifactPage1.SnapshotSeq)
	}
	if artifactPage2.NextAfter != "" {
		t.Fatalf("ListRunArtifacts(page2) next_after = %q, want empty", artifactPage2.NextAfter)
	}

	if _, err := svc.ResolveCheckpoint(ctx, caller, checkpointIDs[0], CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "continue",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}

	historicalCheckpoints, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{
		Limit:   10,
		AsOfSeq: checkpointPage1.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunCheckpoints(historical) error = %v", err)
	}
	for _, item := range historicalCheckpoints.Items {
		if item.Status != CheckpointStatusOpen {
			t.Fatalf("historical checkpoint %q status = %q, want %q", item.ID, item.Status, CheckpointStatusOpen)
		}
	}

	historicalArtifacts, err := svc.ListRunArtifacts(ctx, caller, handle.RunID, ListRunArtifactsRequest{
		Limit:   10,
		AsOfSeq: artifactPage1.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunArtifacts(historical) error = %v", err)
	}
	if len(historicalArtifacts.Items) != 3 {
		t.Fatalf("historical artifacts len = %d, want 3", len(historicalArtifacts.Items))
	}
}

func TestIntegrationListRunEventsSupportsStableReplayPagination(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "events replay should be stable across after_seq pages",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	createResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "continue?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "continue", Kind: CheckpointOptionKindChoice, Label: "Continue"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}
	resolveResult, err := svc.ResolveCheckpoint(ctx, caller, createResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "continue",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}

	page1, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{
		Limit:    2,
		UntilSeq: resolveResult.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunEvents(page1) error = %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("ListRunEvents(page1) len = %d, want 2", len(page1.Items))
	}
	if page1.UntilSeq != resolveResult.SnapshotSeq {
		t.Fatalf("ListRunEvents(page1) until_seq = %d, want %d", page1.UntilSeq, resolveResult.SnapshotSeq)
	}

	lastSeq := page1.Items[len(page1.Items)-1].Seq
	page2, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{
		AfterSeq: lastSeq,
		Limit:    100,
		UntilSeq: resolveResult.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunEvents(page2) error = %v", err)
	}
	if page2.UntilSeq != page1.UntilSeq {
		t.Fatalf("ListRunEvents(page2) until_seq = %d, want %d", page2.UntilSeq, page1.UntilSeq)
	}
	for _, item := range page2.Items {
		if item.Seq <= lastSeq {
			t.Fatalf("event seq %d on page2 should be greater than %d", item.Seq, lastSeq)
		}
		if item.Seq > resolveResult.SnapshotSeq {
			t.Fatalf("event seq %d exceeds until_seq %d", item.Seq, resolveResult.SnapshotSeq)
		}
	}
}

func TestIntegrationListRunEventsContinuationRequiresUntilSeq(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "event continuation should require until_seq",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	page1, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListRunEvents(page1) error = %v", err)
	}
	if len(page1.Items) != 1 {
		t.Fatalf("ListRunEvents(page1) len = %d, want 1", len(page1.Items))
	}
	if _, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{
		AfterSeq: page1.Items[0].Seq,
		Limit:    10,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ListRunEvents(continuation without until_seq) error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestIntegrationSnapshotSeqCutsAcrossAllReadModelsConsistently(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "all read models should agree on the same snapshot cut",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	siblingTaskID := createIntegrationChildTask(t, ctx, svc, handle.RunID, "sibling", "run.slice.sibling")

	artifactResult, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Kind:           "report",
		URI:            "memoh://artifact/before-checkpoint.md",
		Version:        "v1",
		Digest:         "sha256:before-checkpoint",
		ContentType:    "text/markdown",
		Summary:        "before checkpoint",
		IdempotencyKey: "artifact-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CommitArtifact() error = %v", err)
	}

	createResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause all work?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "resume", Kind: CheckpointOptionKindChoice, Label: "Resume"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}
	if createResult.SnapshotSeq <= artifactResult.SnapshotSeq {
		t.Fatalf("checkpoint snapshot_seq = %d, want > artifact snapshot_seq %d", createResult.SnapshotSeq, artifactResult.SnapshotSeq)
	}

	snapshot, err := svc.GetRunSnapshotAtSeq(ctx, caller, handle.RunID, createResult.SnapshotSeq)
	if err != nil {
		t.Fatalf("GetRunSnapshotAtSeq() error = %v", err)
	}
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10, AsOfSeq: createResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	checkpointPage, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10, AsOfSeq: createResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	artifactPage, err := svc.ListRunArtifacts(ctx, caller, handle.RunID, ListRunArtifactsRequest{Limit: 10, AsOfSeq: createResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 100, UntilSeq: createResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}

	if snapshot.SnapshotSeq != createResult.SnapshotSeq {
		t.Fatalf("snapshot_seq = %d, want %d", snapshot.SnapshotSeq, createResult.SnapshotSeq)
	}
	if taskPage.SnapshotSeq != createResult.SnapshotSeq {
		t.Fatalf("task page snapshot_seq = %d, want %d", taskPage.SnapshotSeq, createResult.SnapshotSeq)
	}
	if checkpointPage.SnapshotSeq != createResult.SnapshotSeq {
		t.Fatalf("checkpoint page snapshot_seq = %d, want %d", checkpointPage.SnapshotSeq, createResult.SnapshotSeq)
	}
	if artifactPage.SnapshotSeq != createResult.SnapshotSeq {
		t.Fatalf("artifact page snapshot_seq = %d, want %d", artifactPage.SnapshotSeq, createResult.SnapshotSeq)
	}
	if eventPage.UntilSeq != createResult.SnapshotSeq {
		t.Fatalf("event page until_seq = %d, want %d", eventPage.UntilSeq, createResult.SnapshotSeq)
	}

	if snapshot.Run.LifecycleStatus != LifecycleStatusWaitingHuman {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusWaitingHuman)
	}
	if len(checkpointPage.Items) != 1 || checkpointPage.Items[0].Status != CheckpointStatusOpen {
		t.Fatalf("checkpoint page = %+v, want one open checkpoint", checkpointPage.Items)
	}
	if len(artifactPage.Items) != 1 || artifactPage.Items[0].ID != artifactResult.Artifact.ID {
		t.Fatalf("artifact page = %+v, want artifact %q", artifactPage.Items, artifactResult.Artifact.ID)
	}

	taskByID := make(map[string]Task, len(taskPage.Items))
	for _, task := range taskPage.Items {
		taskByID[task.ID] = task
	}
	if len(taskByID) != 2 {
		t.Fatalf("task page len = %d, want 2", len(taskByID))
	}
	if taskByID[handle.RootTaskID].Status != TaskStatusWaitingHuman || taskByID[handle.RootTaskID].WaitingScope != "task" {
		t.Fatalf("root task at cut = %+v, want waiting_human/task", taskByID[handle.RootTaskID])
	}
	if taskByID[siblingTaskID].Status != TaskStatusWaitingHuman || taskByID[siblingTaskID].WaitingScope != "run" {
		t.Fatalf("sibling task at cut = %+v, want waiting_human/run", taskByID[siblingTaskID])
	}

	var sawArtifactCommitted bool
	var sawCheckpointRequested bool
	var sawCheckpointResolved bool
	for _, event := range eventPage.Items {
		switch event.Type {
		case "run.event.artifact.committed":
			sawArtifactCommitted = true
		case "run.event.hitl.requested":
			sawCheckpointRequested = true
		case "run.event.hitl.resolved":
			sawCheckpointResolved = true
		}
	}
	if !sawArtifactCommitted {
		t.Fatal("event page missing run.event.artifact.committed")
	}
	if !sawCheckpointRequested {
		t.Fatal("event page missing run.event.hitl.requested")
	}
	if sawCheckpointResolved {
		t.Fatal("event page unexpectedly included run.event.hitl.resolved at create snapshot")
	}
}

func TestIntegrationCommitArtifactHidesForeignRunTaskAndAttempt(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	subject := "subject-" + uuid.NewString()
	caller := ControlIdentity{
		TenantID: "tenant-a-" + uuid.NewString(),
		Subject:  subject,
	}
	otherTenantCaller := ControlIdentity{
		TenantID: "tenant-b-" + uuid.NewString(),
		Subject:  subject,
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "artifact auth should be hidden",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, otherTenantCaller.TenantID, otherTenantCaller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	if _, err := svc.CommitArtifact(ctx, otherTenantCaller, CommitArtifactRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		AttemptID:      attempt.ID,
		Kind:           "report",
		URI:            "memoh://artifacts/hidden",
		Version:        "v1",
		Digest:         "sha256:hidden",
		IdempotencyKey: "artifact-foreign-run-" + uuid.NewString(),
	}); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("CommitArtifact(cross-tenant same-subject) error = %v, want %v", err, ErrRunNotFound)
	}

	secondHandle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "second run for foreign nested resources",
		IdempotencyKey: "start-second-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun(second) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, secondHandle.RunID)

	if _, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          secondHandle.RunID,
		TaskID:         handle.RootTaskID,
		AttemptID:      attempt.ID,
		Kind:           "report",
		URI:            "memoh://artifacts/foreign-task",
		Version:        "v1",
		Digest:         "sha256:foreign-task",
		IdempotencyKey: "artifact-foreign-task-" + uuid.NewString(),
	}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("CommitArtifact(foreign task) error = %v, want %v", err, ErrTaskNotFound)
	}

	if _, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          secondHandle.RunID,
		TaskID:         secondHandle.RootTaskID,
		AttemptID:      attempt.ID,
		Kind:           "report",
		URI:            "memoh://artifacts/foreign-attempt",
		Version:        "v1",
		Digest:         "sha256:foreign-attempt",
		IdempotencyKey: "artifact-foreign-attempt-" + uuid.NewString(),
	}); !errors.Is(err, ErrAttemptNotFound) {
		t.Fatalf("CommitArtifact(foreign attempt) error = %v, want %v", err, ErrAttemptNotFound)
	}
}

func TestIntegrationCreateHumanCheckpointIdempotencyCanonicalizesUUIDCase(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "checkpoint idempotency should ignore UUID case",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	idempotencyKey := "checkpoint-" + uuid.NewString()
	first, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          strings.ToLower(handle.RunID),
		TaskID:         strings.ToLower(handle.RootTaskID),
		Question:       "canonical?",
		IdempotencyKey: idempotencyKey,
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(first) error = %v", err)
	}

	replayed, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          strings.ToUpper(handle.RunID),
		TaskID:         strings.ToUpper(handle.RootTaskID),
		Question:       "canonical?",
		IdempotencyKey: idempotencyKey,
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(replay uppercase) error = %v", err)
	}
	if replayed.Checkpoint.ID != first.Checkpoint.ID {
		t.Fatalf("CreateHumanCheckpoint(replay) checkpoint_id = %q, want %q", replayed.Checkpoint.ID, first.Checkpoint.ID)
	}
}

func TestIntegrationCommitArtifactIdempotencyCanonicalizesUUIDCase(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "artifact idempotency should ignore UUID case",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	idempotencyKey := "artifact-" + uuid.NewString()
	first, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          strings.ToLower(handle.RunID),
		TaskID:         strings.ToLower(handle.RootTaskID),
		Kind:           "report",
		URI:            "memoh://artifact/canonical",
		Version:        "v1",
		Digest:         "sha256:canonical",
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("CommitArtifact(first) error = %v", err)
	}

	replayed, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          strings.ToUpper(handle.RunID),
		TaskID:         strings.ToUpper(handle.RootTaskID),
		Kind:           "report",
		URI:            "memoh://artifact/canonical",
		Version:        "v1",
		Digest:         "sha256:canonical",
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("CommitArtifact(replay uppercase) error = %v", err)
	}
	if replayed.Artifact.ID != first.Artifact.ID {
		t.Fatalf("CommitArtifact(replay) artifact_id = %q, want %q", replayed.Artifact.ID, first.Artifact.ID)
	}
}

func TestIntegrationCommitArtifactEventCanonicalizesUUIDPayloads(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "artifact events should canonicalize UUID payloads",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	result, err := svc.CommitArtifact(ctx, caller, CommitArtifactRequest{
		RunID:          strings.ToUpper(handle.RunID),
		TaskID:         strings.ToUpper(handle.RootTaskID),
		Kind:           "report",
		URI:            "memoh://artifact/canonicalize.md",
		Version:        "v1",
		Digest:         "sha256:canonicalize",
		ContentType:    "text/markdown",
		Summary:        "canonicalize event ids",
		IdempotencyKey: "artifact-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CommitArtifact() error = %v", err)
	}

	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{
		Limit:    20,
		UntilSeq: result.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}

	var committed *RunEvent
	for idx := range eventPage.Items {
		if eventPage.Items[idx].Type == "run.event.artifact.committed" && eventPage.Items[idx].Payload["artifact_id"] == result.Artifact.ID {
			committed = &eventPage.Items[idx]
			break
		}
	}
	if committed == nil {
		t.Fatal("artifact committed event not found")
	}
	if committed.Payload["run_id"] != handle.RunID {
		t.Fatalf("artifact committed event run_id = %v, want %q", committed.Payload["run_id"], handle.RunID)
	}
	if committed.Payload["task_id"] != handle.RootTaskID {
		t.Fatalf("artifact committed event task_id = %v, want %q", committed.Payload["task_id"], handle.RootTaskID)
	}
}

func TestIntegrationRunBlockingCheckpointPausesAndResumesSiblingTasks(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "pause sibling tasks on global checkpoint",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "sibling task",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusReady,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause everything?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "resume", Kind: CheckpointOptionKindChoice, Label: "Resume"},
		},
		ResumePolicy: &CheckpointResumePolicy{ResumeMode: " " + CheckpointResumeModeNewAttempt + " "},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after pause) error = %v", err)
	}
	if len(taskPage.Items) != 2 {
		t.Fatalf("ListRunTasks(after pause) len = %d, want 2", len(taskPage.Items))
	}
	var siblingPaused bool
	for _, task := range taskPage.Items {
		if task.ID == handle.RootTaskID {
			continue
		}
		if task.Status == TaskStatusWaitingHuman && task.WaitingScope == "run" && task.WaitingCheckpointID == checkpointResult.Checkpoint.ID {
			siblingPaused = true
		}
	}
	if !siblingPaused {
		t.Fatal("sibling task was not paused by run-blocking checkpoint")
	}

	resolveID := "resolve-" + uuid.NewString()
	resolveResult, err := svc.ResolveCheckpoint(ctx, caller, strings.ToUpper(checkpointResult.Checkpoint.ID), CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "resume",
		IdempotencyKey: resolveID + " ",
	})
	if err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	if _, err := svc.ResolveCheckpoint(ctx, caller, checkpointResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "resume",
		IdempotencyKey: resolveID,
	}); err != nil {
		t.Fatalf("ResolveCheckpoint(idempotent retry) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after resume) error = %v", err)
	}
	for _, task := range taskPage.Items {
		if task.Status != TaskStatusReady {
			t.Fatalf("task %q status after resume = %q, want %q", task.ID, task.Status, TaskStatusReady)
		}
	}

	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{
		Limit:    100,
		UntilSeq: resolveResult.SnapshotSeq,
	})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	var foundResolved bool
	for _, event := range eventPage.Items {
		if event.Type != "run.event.hitl.resolved" {
			continue
		}
		blocksRun, ok := event.Payload["blocks_run"].(bool)
		if !ok {
			t.Fatalf("resolved event blocks_run type = %T, want bool", event.Payload["blocks_run"])
		}
		if !blocksRun {
			t.Fatal("resolved event blocks_run = false, want true")
		}
		foundResolved = true
	}
	if !foundResolved {
		t.Fatal("ListRunEvents() missing run.event.hitl.resolved for run-blocking checkpoint")
	}
}

func TestIntegrationChildTaskFailureFailsRun(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "child task failure should fail the run",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, childTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(child) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   childTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "failing child task",
		Inputs:               marshalObject(map[string]any{"kind": "child"}),
		PlannerEpoch:         1,
		WorkerProfile:        DefaultRootWorkerProfile,
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusReady,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.child",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(child) error = %v", err)
	}

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:      runningAttempt.ID,
		ClaimToken:     runningAttempt.ClaimToken,
		Status:         TaskAttemptStatusFailed,
		FailureClass:   "integration_failure",
		TerminalReason: "child task failed",
		Summary:        "child task failed",
	}); err != nil {
		t.Fatalf("CompleteAttempt(failed) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusFailed {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusFailed)
	}
}

func TestIntegrationAttemptLeaseExpiryIsStrictlyFenced(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "expired attempt lease should be fenced",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}

	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET lease_expires_at = now() - interval '1 second' WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("expire attempt lease: %v", err)
	}

	if _, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken); !errors.Is(err, ErrAttemptLeaseConflict) {
		t.Fatalf("StartAttempt(expired lease) error = %v, want %v", err, ErrAttemptLeaseConflict)
	}
	if _, err := svc.HeartbeatAttempt(ctx, AttemptHeartbeat{
		AttemptID:       attempt.ID,
		ClaimToken:      attempt.ClaimToken,
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrAttemptLeaseConflict) {
		t.Fatalf("HeartbeatAttempt(expired lease) error = %v, want %v", err, ErrAttemptLeaseConflict)
	}
}

func TestIntegrationCompleteAttemptRejectsExpiredLeaseAndRecoveryFailsRunOnce(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "expired running attempt should be recovered once",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}

	pgAttemptID, err := db.ParseUUID(runningAttempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET lease_expires_at = now() - interval '1 second' WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("expire running attempt lease: %v", err)
	}

	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "should be fenced",
		StructuredOutput: map[string]any{"ok": true},
	}); !errors.Is(err, ErrAttemptLeaseConflict) {
		t.Fatalf("CompleteAttempt(expired lease) error = %v, want %v", err, ErrAttemptLeaseConflict)
	}

	recovered, err := svc.RecoverExpiredAttempts(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredAttempts(first) error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverExpiredAttempts(first) = %d, want 1", recovered)
	}
	recovered, err = svc.RecoverExpiredAttempts(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredAttempts(second) error = %v", err)
	}
	if recovered != 0 {
		t.Fatalf("RecoverExpiredAttempts(second) = %d, want 0", recovered)
	}

	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusLost {
		t.Fatalf("attempt row status = %q, want %q", attemptRow.Status, TaskAttemptStatusLost)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusFailed {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusFailed)
	}
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10, AsOfSeq: snapshot.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 || taskPage.Items[0].Status != TaskStatusFailed {
		t.Fatalf("task page status = %+v, want single failed task", taskPage.Items)
	}
}

func TestIntegrationStartAttemptEmitsBindingBeforeRunning(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "attempt start should emit binding before running",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if runningAttempt.Status != TaskAttemptStatusRunning {
		t.Fatalf("StartAttempt() status = %q, want %q", runningAttempt.Status, TaskAttemptStatusRunning)
	}

	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	bindingIndex := -1
	runningIndex := -1
	for i, event := range eventPage.Items {
		switch event.Type {
		case "run.event.attempt.binding":
			if attemptID, _ := event.Payload["attempt_id"].(string); attemptID == attempt.ID {
				bindingIndex = i
			}
		case "run.event.attempt.running":
			if attemptID, _ := event.Payload["attempt_id"].(string); attemptID == attempt.ID {
				runningIndex = i
			}
		}
	}
	if bindingIndex < 0 {
		t.Fatal("ListRunEvents() missing run.event.attempt.binding")
	}
	if runningIndex < 0 {
		t.Fatal("ListRunEvents() missing run.event.attempt.running")
	}
	if bindingIndex >= runningIndex {
		t.Fatalf("attempt binding/running event order = (%d, %d), want binding before running", bindingIndex, runningIndex)
	}
}

func TestIntegrationStartAttemptReplayPreservesRunningAttempt(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "start attempt replay should preserve running attempt",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(first) error = %v", err)
	}
	replayedAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(replay) error = %v", err)
	}
	if replayedAttempt.Status != TaskAttemptStatusRunning {
		t.Fatalf("StartAttempt(replay) status = %q, want %q", replayedAttempt.Status, TaskAttemptStatusRunning)
	}
	if replayedAttempt.ID != runningAttempt.ID {
		t.Fatalf("StartAttempt(replay) id = %q, want %q", replayedAttempt.ID, runningAttempt.ID)
	}

	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusRunning {
		t.Fatalf("attempt row status after replay = %q, want %q", attemptRow.Status, TaskAttemptStatusRunning)
	}
	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	bindingCount := 0
	runningCount := 0
	taskRunningCount := 0
	for _, event := range eventPage.Items {
		attemptID, _ := event.Payload["attempt_id"].(string)
		if attemptID != attempt.ID {
			continue
		}
		switch event.Type {
		case "run.event.attempt.binding":
			bindingCount++
		case "run.event.attempt.running":
			runningCount++
		case "run.event.task.running":
			taskRunningCount++
		}
	}
	if bindingCount != 1 || runningCount != 1 || taskRunningCount != 1 {
		t.Fatalf("replay emitted duplicate events: binding=%d running=%d task_running=%d, want 1/1/1", bindingCount, runningCount, taskRunningCount)
	}
}

func TestIntegrationBindingAttemptHeartbeatAndRecovery(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "binding attempt should heartbeat and recover",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET status = 'binding', updated_at = now() WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("mark attempt binding: %v", err)
	}

	heartbeatAttempt, err := svc.HeartbeatAttempt(ctx, AttemptHeartbeat{
		AttemptID:       attempt.ID,
		ClaimToken:      attempt.ClaimToken,
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("HeartbeatAttempt(binding) error = %v", err)
	}
	if heartbeatAttempt.Status != TaskAttemptStatusBinding {
		t.Fatalf("HeartbeatAttempt(binding) status = %q, want %q", heartbeatAttempt.Status, TaskAttemptStatusBinding)
	}

	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("expire binding attempt lease: %v", err)
	}

	recovered, err := svc.RecoverExpiredAttempts(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredAttempts() error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverExpiredAttempts() = %d, want 1", recovered)
	}

	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusCreated {
		t.Fatalf("attempt row status = %q, want %q", attemptRow.Status, TaskAttemptStatusCreated)
	}
	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	var foundRequeued bool
	for _, event := range eventPage.Items {
		if event.Type != "run.event.attempt.requeued" {
			continue
		}
		if attemptID, _ := event.Payload["attempt_id"].(string); attemptID == attempt.ID {
			foundRequeued = true
			break
		}
	}
	if !foundRequeued {
		t.Fatal("ListRunEvents() missing run.event.attempt.requeued for recovered binding attempt")
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusRunning {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusRunning)
	}
}

func TestIntegrationStartAttemptRetiresClaimWhenRunBecomesImmutable(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "claim should be released when run becomes immutable",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_runs SET lifecycle_status = 'failed', updated_at = now() WHERE id = $1", pgRunID); err != nil {
		t.Fatalf("mark run failed: %v", err)
	}

	if _, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken); !errors.Is(err, ErrAttemptImmutable) {
		t.Fatalf("StartAttempt(immutable run) error = %v, want %v", err, ErrAttemptImmutable)
	}

	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusFailed {
		t.Fatalf("attempt status after retirement = %q, want %q", attemptRow.Status, TaskAttemptStatusFailed)
	}
	if attemptRow.FailureClass != "attempt_immutable" {
		t.Fatalf("attempt failure_class after retirement = %q, want %q", attemptRow.FailureClass, "attempt_immutable")
	}
	if attemptRow.TerminalReason != "attempt invalidated before start" {
		t.Fatalf("attempt terminal_reason after retirement = %q, want %q", attemptRow.TerminalReason, "attempt invalidated before start")
	}
	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	var foundFailed bool
	for _, event := range eventPage.Items {
		if event.Type != "run.event.attempt.failed" {
			continue
		}
		if attemptID, _ := event.Payload["attempt_id"].(string); attemptID == attempt.ID {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Fatal("ListRunEvents() missing run.event.attempt.failed for immutable run retirement")
	}
}

func TestIntegrationStartAttemptAdvancesPersistedBindingToRunning(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "persisted binding should advance to running",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET status = 'binding', updated_at = now() WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("mark attempt binding: %v", err)
	}

	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(binding replay) error = %v", err)
	}
	if runningAttempt.Status != TaskAttemptStatusRunning {
		t.Fatalf("StartAttempt(binding replay) status = %q, want %q", runningAttempt.Status, TaskAttemptStatusRunning)
	}
}

func TestIntegrationStartAttemptRetiresBoundAttemptWhenRunBecomesImmutable(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "bound attempt should retire when run becomes immutable",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET status = 'binding', updated_at = now() WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("mark attempt binding: %v", err)
	}

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_runs SET lifecycle_status = 'failed', updated_at = now() WHERE id = $1", pgRunID); err != nil {
		t.Fatalf("mark run failed: %v", err)
	}

	if _, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken); !errors.Is(err, ErrAttemptImmutable) {
		t.Fatalf("StartAttempt(bound immutable run) error = %v, want %v", err, ErrAttemptImmutable)
	}

	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusFailed {
		t.Fatalf("attempt status after bound immutable retirement = %q, want %q", attemptRow.Status, TaskAttemptStatusFailed)
	}
}

func TestIntegrationStartAttemptRetiresClaimWhenTaskBecomesImmutable(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "claim should retire when task becomes immutable",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := svc.DispatchNextReadyTask(ctx)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}

	pgTaskID, err := db.ParseUUID(handle.RootTaskID)
	if err != nil {
		t.Fatalf("parse task uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_tasks SET status = 'blocked', updated_at = now() WHERE id = $1", pgTaskID); err != nil {
		t.Fatalf("mark task blocked: %v", err)
	}

	if _, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken); !errors.Is(err, ErrAttemptImmutable) {
		t.Fatalf("StartAttempt(immutable task) error = %v, want %v", err, ErrAttemptImmutable)
	}

	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusFailed {
		t.Fatalf("attempt status after immutable task retirement = %q, want %q", attemptRow.Status, TaskAttemptStatusFailed)
	}
	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	var foundFailed bool
	for _, event := range eventPage.Items {
		if event.Type != "run.event.attempt.failed" {
			continue
		}
		if attemptID, _ := event.Payload["attempt_id"].(string); attemptID == attempt.ID {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Fatal("ListRunEvents() missing run.event.attempt.failed for immutable task retirement")
	}
}

func TestIntegrationStalePlanningIntentFailsWithoutMutatingRunOrTask(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "stale planning intent should fail closed",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	pgTaskID, err := db.ParseUUID(handle.RootTaskID)
	if err != nil {
		t.Fatalf("parse task uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_runs SET planner_epoch = planner_epoch + 1, updated_at = now() WHERE id = $1", pgRunID); err != nil {
		t.Fatalf("bump planner_epoch: %v", err)
	}

	_, intentUUID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(intent) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               intentUUID,
		RunID:            pgRunID,
		TaskID:           pgTaskID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindStartRun,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: 1,
		Payload:          marshalJSON(map[string]any{"reason": "stale-test"}),
	}); err != nil {
		t.Fatalf("CreateOrchestrationPlanningIntent() error = %v", err)
	}

	processed, err := svc.ProcessNextPlanningIntent(ctx)
	if err != nil {
		t.Fatalf("ProcessNextPlanningIntent() error = %v", err)
	}
	if !processed {
		t.Fatal("ProcessNextPlanningIntent() = false, want true")
	}

	intentRow, err := svc.queries.GetOrchestrationPlanningIntentByID(ctx, intentUUID)
	if err != nil {
		t.Fatalf("GetOrchestrationPlanningIntentByID() error = %v", err)
	}
	if intentRow.Status != PlanningIntentStatusFailed {
		t.Fatalf("planning intent status = %q, want %q", intentRow.Status, PlanningIntentStatusFailed)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusRunning {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusRunning)
	}
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10, AsOfSeq: snapshot.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 || taskPage.Items[0].Status != TaskStatusReady {
		t.Fatalf("task page = %+v, want single ready task", taskPage.Items)
	}

	eventPage, err := svc.ListRunEvents(ctx, caller, handle.RunID, ListRunEventsRequest{Limit: 100})
	if err != nil {
		t.Fatalf("ListRunEvents() error = %v", err)
	}
	var found bool
	for _, event := range eventPage.Items {
		if event.Type != "run.event.planning_status.changed" {
			continue
		}
		found = true
	}
	if !found {
		t.Fatal("ListRunEvents() missing run.event.planning_status.changed after stale intent failure")
	}
}

func TestIntegrationOrphanedPlanningIntentFailsAndDoesNotLoopUntilLeaseExpiry(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "orphaned planning intent should fail immediately",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	pgTaskID, err := db.ParseUUID(handle.RootTaskID)
	if err != nil {
		t.Fatalf("parse task uuid: %v", err)
	}
	_, intentUUID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(intent) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               intentUUID,
		RunID:            pgRunID,
		TaskID:           pgTaskID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindStartRun,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: 1,
		Payload:          marshalJSON(map[string]any{"reason": "orphaned-run-test"}),
	}); err != nil {
		t.Fatalf("CreateOrchestrationPlanningIntent() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM orchestration_runs WHERE id = $1", pgRunID); err != nil {
		t.Fatalf("delete orchestration run: %v", err)
	}

	var remainingIntentCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM orchestration_planning_intents WHERE id = $1", intentUUID).Scan(&remainingIntentCount); err != nil {
		t.Fatalf("query planning intent after run delete: %v", err)
	}
	if remainingIntentCount != 0 {
		t.Fatalf("planning intent row count after run delete = %d, want 0", remainingIntentCount)
	}

	processed, err := svc.ProcessNextPlanningIntent(ctx)
	if err != nil {
		t.Fatalf("ProcessNextPlanningIntent() error = %v", err)
	}
	if processed {
		t.Fatal("ProcessNextPlanningIntent() = true, want false")
	}

	processed, err = svc.ProcessNextPlanningIntent(ctx)
	if err != nil {
		t.Fatalf("ProcessNextPlanningIntent(second) error = %v", err)
	}
	if processed {
		t.Fatal("ProcessNextPlanningIntent(second) = true, want false")
	}
}

func TestIntegrationWorkerLeaseTokenFencesSplitBrainHeartbeat(t *testing.T) {
	svc, _, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	workerID := "worker-" + uuid.NewString()

	primaryLease, err := svc.RegisterWorker(ctx, WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      DefaultWorkerExecutorID,
		DisplayName:     workerID,
		Capabilities:    map[string]any{"worker_profiles": []any{DefaultRootWorkerProfile}},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("RegisterWorker(primary) error = %v", err)
	}
	if strings.TrimSpace(primaryLease.LeaseToken) == "" {
		t.Fatal("RegisterWorker(primary) lease_token = empty, want non-empty")
	}

	if _, err := svc.RegisterWorker(ctx, WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      DefaultWorkerExecutorID,
		DisplayName:     workerID,
		Capabilities:    map[string]any{"worker_profiles": []any{DefaultRootWorkerProfile}},
		LeaseToken:      "conflicting-" + uuid.NewString(),
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrWorkerLeaseConflict) {
		t.Fatalf("RegisterWorker(conflicting) error = %v, want %v", err, ErrWorkerLeaseConflict)
	}

	if _, err := svc.HeartbeatWorker(ctx, workerID, primaryLease.LeaseToken, 30); err != nil {
		t.Fatalf("HeartbeatWorker(primary) error = %v", err)
	}
	if _, err := svc.HeartbeatWorker(ctx, workerID, "stale-"+uuid.NewString(), 30); !errors.Is(err, ErrWorkerLeaseConflict) {
		t.Fatalf("HeartbeatWorker(stale token) error = %v, want %v", err, ErrWorkerLeaseConflict)
	}
}

func TestIntegrationClaimedAttemptRecoveryRequeuesInsteadOfFailing(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "claimed attempt should be requeued on lease expiry",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	attempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	}, 4)
	pgAttemptID, err := db.ParseUUID(attempt.ID)
	if err != nil {
		t.Fatalf("parse attempt uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_attempts SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", pgAttemptID); err != nil {
		t.Fatalf("expire claimed attempt lease: %v", err)
	}

	recovered, err := svc.RecoverExpiredAttempts(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredAttempts() error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverExpiredAttempts() = %d, want 1", recovered)
	}

	attemptRow, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, pgAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID() error = %v", err)
	}
	if attemptRow.Status != TaskAttemptStatusCreated {
		t.Fatalf("attempt status = %q, want %q", attemptRow.Status, TaskAttemptStatusCreated)
	}
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 || taskPage.Items[0].Status != TaskStatusDispatching {
		t.Fatalf("task page = %+v, want single dispatching task", taskPage.Items)
	}
	reclaimed, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(reclaimed) error = %v", err)
	}
	if reclaimed.ID != attempt.ID {
		t.Fatalf("reclaimed attempt id = %q, want %q", reclaimed.ID, attempt.ID)
	}
}

func TestIntegrationClaimedVerificationRecoveryRequeuesInsteadOfFailing(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "claimed verification should be requeued on lease expiry",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	attempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	}, 4)
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "ready for verification",
		StructuredOutput: map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(verifying) error = %v", err)
	}
	if len(taskPage.Items) != 1 || taskPage.Items[0].Status != TaskStatusVerifying {
		t.Fatalf("task page = %+v, want single verifying task", taskPage.Items)
	}

	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	pgVerificationID, err := db.ParseUUID(verification.ID)
	if err != nil {
		t.Fatalf("parse verification uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_task_verifications SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", pgVerificationID); err != nil {
		t.Fatalf("expire claimed verification lease: %v", err)
	}

	recovered, err := svc.RecoverExpiredVerifications(ctx)
	if err != nil {
		t.Fatalf("RecoverExpiredVerifications() error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverExpiredVerifications() = %d, want 1", recovered)
	}

	verificationRow, err := svc.queries.GetOrchestrationTaskVerificationByID(ctx, pgVerificationID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskVerificationByID() error = %v", err)
	}
	if verificationRow.Status != TaskVerificationStatusCreated {
		t.Fatalf("verification status = %q, want %q", verificationRow.Status, TaskVerificationStatusCreated)
	}
	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 || taskPage.Items[0].Status != TaskStatusVerifying {
		t.Fatalf("task page = %+v, want single verifying task", taskPage.Items)
	}
	reclaimed, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification(reclaimed) error = %v", err)
	}
	if reclaimed.ID != verification.ID {
		t.Fatalf("reclaimed verification id = %q, want %q", reclaimed.ID, verification.ID)
	}
}

func TestIntegrationStaleWorkerTakeoverFencesClaimedAttemptLeaseOps(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "stale claimed attempt should be fenced after worker takeover",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	workerID := "worker-" + uuid.NewString()
	oldLease := "lease-old-" + uuid.NewString()
	attempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        workerID,
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseToken:      oldLease,
		LeaseTTLSeconds: 30,
	}, 4)
	if _, err := pool.Exec(ctx, "UPDATE orchestration_workers SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", workerID); err != nil {
		t.Fatalf("expire worker lease: %v", err)
	}
	if _, err := svc.RegisterWorker(ctx, WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      DefaultWorkerExecutorID,
		DisplayName:     workerID,
		Capabilities:    workerCapabilities([]string{DefaultRootWorkerProfile}),
		LeaseToken:      "lease-new-" + uuid.NewString(),
		LeaseTTLSeconds: 30,
	}); err != nil {
		t.Fatalf("RegisterWorker(takeover) error = %v", err)
	}

	if _, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken); !errors.Is(err, ErrAttemptLeaseConflict) {
		t.Fatalf("StartAttempt(stale worker) error = %v, want %v", err, ErrAttemptLeaseConflict)
	}
	if _, err := svc.HeartbeatAttempt(ctx, AttemptHeartbeat{
		AttemptID:       attempt.ID,
		ClaimToken:      attempt.ClaimToken,
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrAttemptLeaseConflict) {
		t.Fatalf("HeartbeatAttempt(stale worker) error = %v, want %v", err, ErrAttemptLeaseConflict)
	}
}

func TestIntegrationStaleWorkerTakeoverFencesRunningAttemptCompletion(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "stale running attempt should be fenced after worker takeover",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	workerID := "worker-" + uuid.NewString()
	oldLease := "lease-old-" + uuid.NewString()
	attempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        workerID,
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseToken:      oldLease,
		LeaseTTLSeconds: 30,
	}, 4)
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_workers SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", workerID); err != nil {
		t.Fatalf("expire worker lease: %v", err)
	}
	if _, err := svc.RegisterWorker(ctx, WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      DefaultWorkerExecutorID,
		DisplayName:     workerID,
		Capabilities:    workerCapabilities([]string{DefaultRootWorkerProfile}),
		LeaseToken:      "lease-new-" + uuid.NewString(),
		LeaseTTLSeconds: 30,
	}); err != nil {
		t.Fatalf("RegisterWorker(takeover) error = %v", err)
	}

	if _, err := svc.HeartbeatAttempt(ctx, AttemptHeartbeat{
		AttemptID:       runningAttempt.ID,
		ClaimToken:      runningAttempt.ClaimToken,
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrAttemptLeaseConflict) {
		t.Fatalf("HeartbeatAttempt(stale running worker) error = %v, want %v", err, ErrAttemptLeaseConflict)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "should be fenced",
		StructuredOutput: map[string]any{"ok": true},
	}); !errors.Is(err, ErrAttemptLeaseConflict) {
		t.Fatalf("CompleteAttempt(stale running worker) error = %v, want %v", err, ErrAttemptLeaseConflict)
	}
}

func TestIntegrationStaleWorkerTakeoverFencesVerificationLeaseOps(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "stale verification should be fenced after worker takeover",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	attempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	}, 4)
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "ready for verification",
		StructuredOutput: map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(verifying) error = %v", err)
	}
	if len(taskPage.Items) != 1 || taskPage.Items[0].Status != TaskStatusVerifying {
		t.Fatalf("task page = %+v, want single verifying task", taskPage.Items)
	}

	workerID := "verifier-" + uuid.NewString()
	oldLease := "lease-old-" + uuid.NewString()
	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         workerID,
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseToken:       oldLease,
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_workers SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", workerID); err != nil {
		t.Fatalf("expire verifier worker lease: %v", err)
	}
	if _, err := svc.RegisterWorker(ctx, WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      DefaultVerifierExecutorID,
		DisplayName:     workerID,
		Capabilities:    verifierCapabilities([]string{DefaultVerifierProfile}),
		LeaseToken:      "lease-new-" + uuid.NewString(),
		LeaseTTLSeconds: 30,
	}); err != nil {
		t.Fatalf("RegisterWorker(takeover) error = %v", err)
	}

	if _, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken); !errors.Is(err, ErrVerificationLeaseConflict) {
		t.Fatalf("StartVerification(stale worker) error = %v, want %v", err, ErrVerificationLeaseConflict)
	}
	if _, err := svc.HeartbeatVerification(ctx, VerificationHeartbeat{
		VerificationID:  verification.ID,
		ClaimToken:      verification.ClaimToken,
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrVerificationLeaseConflict) {
		t.Fatalf("HeartbeatVerification(stale worker) error = %v, want %v", err, ErrVerificationLeaseConflict)
	}
}

func TestIntegrationStaleWorkerTakeoverFencesRunningVerificationCompletion(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "stale running verification should be fenced after worker takeover",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	setIntegrationTaskVerificationPolicy(t, ctx, pool, handle.RootTaskID, map[string]any{
		"require_structured_output": true,
	})

	attempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	}, 4)
	runningAttempt, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        runningAttempt.ID,
		ClaimToken:       runningAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "ready for verification",
		StructuredOutput: map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(verifying) error = %v", err)
	}
	if len(taskPage.Items) != 1 || taskPage.Items[0].Status != TaskStatusVerifying {
		t.Fatalf("task page = %+v, want single verifying task", taskPage.Items)
	}

	workerID := "verifier-" + uuid.NewString()
	oldLease := "lease-old-" + uuid.NewString()
	verification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         workerID,
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{DefaultVerifierProfile},
		LeaseToken:       oldLease,
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification() error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, verification.ID, verification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_workers SET lease_expires_at = now() - interval '1 second', updated_at = now() WHERE id = $1", workerID); err != nil {
		t.Fatalf("expire verifier worker lease: %v", err)
	}
	if _, err := svc.RegisterWorker(ctx, WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      DefaultVerifierExecutorID,
		DisplayName:     workerID,
		Capabilities:    verifierCapabilities([]string{DefaultVerifierProfile}),
		LeaseToken:      "lease-new-" + uuid.NewString(),
		LeaseTTLSeconds: 30,
	}); err != nil {
		t.Fatalf("RegisterWorker(takeover) error = %v", err)
	}

	if _, err := svc.HeartbeatVerification(ctx, VerificationHeartbeat{
		VerificationID:  runningVerification.ID,
		ClaimToken:      runningVerification.ClaimToken,
		LeaseTTLSeconds: 30,
	}); !errors.Is(err, ErrVerificationLeaseConflict) {
		t.Fatalf("HeartbeatVerification(stale running worker) error = %v, want %v", err, ErrVerificationLeaseConflict)
	}
	if _, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: runningVerification.ID,
		ClaimToken:     runningVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "should be fenced",
	}); !errors.Is(err, ErrVerificationLeaseConflict) {
		t.Fatalf("CompleteVerification(stale running worker) error = %v, want %v", err, ErrVerificationLeaseConflict)
	}
}

func TestIntegrationCheckpointResumeIntentIgnoresPlannerEpochDrift(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "stale checkpoint resume intent should fail closed",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "continue?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "continue", Kind: CheckpointOptionKindChoice, Label: "Continue"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	pgCheckpointID, err := db.ParseUUID(checkpointResult.Checkpoint.ID)
	if err != nil {
		t.Fatalf("parse checkpoint uuid: %v", err)
	}
	resolveResult, err := svc.ResolveCheckpoint(ctx, caller, checkpointResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "continue",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	var intentID pgtype.UUID
	var basePlannerEpoch int64
	if err := pool.QueryRow(ctx, `
SELECT id, base_planner_epoch
FROM orchestration_planning_intents
WHERE run_id = $1
  AND checkpoint_id = $2
  AND kind = $3
ORDER BY created_at DESC
LIMIT 1
`, pgRunID, pgCheckpointID, PlanningIntentKindCheckpointResume).Scan(&intentID, &basePlannerEpoch); err != nil {
		t.Fatalf("load checkpoint_resume planning intent: %v", err)
	}
	if !intentID.Valid {
		t.Fatal("checkpoint_resume planning intent not found")
	}
	if basePlannerEpoch != 1 {
		t.Fatalf("planning intent base_planner_epoch = %d, want 1", basePlannerEpoch)
	}
	if _, err := pool.Exec(ctx, "UPDATE orchestration_runs SET planner_epoch = planner_epoch + 1, updated_at = now() WHERE id = $1", pgRunID); err != nil {
		t.Fatalf("bump planner_epoch: %v", err)
	}

	processed, err := svc.ProcessNextPlanningIntent(ctx)
	if err != nil {
		t.Fatalf("ProcessNextPlanningIntent() error = %v", err)
	}
	if !processed {
		t.Fatal("ProcessNextPlanningIntent() = false, want true")
	}

	intentRow, err := svc.queries.GetOrchestrationPlanningIntentByID(ctx, intentID)
	if err != nil {
		t.Fatalf("GetOrchestrationPlanningIntentByID() error = %v", err)
	}
	if resolveResult.SnapshotSeq == 0 {
		t.Fatal("ResolveCheckpoint() snapshot_seq = 0, want non-zero")
	}
	if intentRow.Status != PlanningIntentStatusCompleted {
		t.Fatalf("planning intent status = %q, want %q", intentRow.Status, PlanningIntentStatusCompleted)
	}

	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks() error = %v", err)
	}
	if len(taskPage.Items) != 1 {
		t.Fatalf("ListRunTasks() len = %d, want 1", len(taskPage.Items))
	}
	if taskPage.Items[0].Status != TaskStatusReady {
		t.Fatalf("task status = %q, want %q", taskPage.Items[0].Status, TaskStatusReady)
	}
	if taskPage.Items[0].WaitingCheckpointID != "" {
		t.Fatalf("waiting checkpoint id = %q, want empty", taskPage.Items[0].WaitingCheckpointID)
	}
}

func TestIntegrationRunBlockingCheckpointSnapshotIsSelfConsistent(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "snapshot seq should fence run barrier state",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "sibling task",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        DefaultRootWorkerProfile,
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusReady,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}

	createResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause everything?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "resume", Kind: CheckpointOptionKindChoice, Label: "Resume"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(after create) error = %v", err)
	}
	if snapshot.SnapshotSeq != createResult.SnapshotSeq {
		t.Fatalf("GetRunSnapshot(after create) snapshot_seq = %d, want %d", snapshot.SnapshotSeq, createResult.SnapshotSeq)
	}
	taskPage, err := svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10, AsOfSeq: createResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunTasks(as_of create) error = %v", err)
	}
	checkpointPage, err := svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10, AsOfSeq: createResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunCheckpoints(as_of create) error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusWaitingHuman {
		t.Fatalf("run lifecycle_status after create = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusWaitingHuman)
	}
	if len(checkpointPage.Items) != 1 || checkpointPage.Items[0].Status != CheckpointStatusOpen {
		t.Fatalf("checkpoint page after create = %+v, want one open checkpoint", checkpointPage.Items)
	}
	if len(taskPage.Items) != 2 {
		t.Fatalf("task page after create len = %d, want 2", len(taskPage.Items))
	}

	resolveResult, err := svc.ResolveCheckpoint(ctx, caller, createResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "resume",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}

	snapshot, err = svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(after resolve) error = %v", err)
	}
	if snapshot.SnapshotSeq != resolveResult.SnapshotSeq {
		t.Fatalf("GetRunSnapshot(after resolve) snapshot_seq = %d, want %d", snapshot.SnapshotSeq, resolveResult.SnapshotSeq)
	}
	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10, AsOfSeq: resolveResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunTasks(as_of resolve) error = %v", err)
	}
	checkpointPage, err = svc.ListRunCheckpoints(ctx, caller, handle.RunID, ListRunCheckpointsRequest{Limit: 10, AsOfSeq: resolveResult.SnapshotSeq})
	if err != nil {
		t.Fatalf("ListRunCheckpoints(as_of resolve) error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusWaitingHuman {
		t.Fatalf("run lifecycle_status at resolve cut = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusWaitingHuman)
	}
	if len(checkpointPage.Items) != 1 || checkpointPage.Items[0].Status != CheckpointStatusResolved {
		t.Fatalf("checkpoint page after resolve = %+v, want one resolved checkpoint", checkpointPage.Items)
	}
	for _, task := range taskPage.Items {
		if task.Status != TaskStatusWaitingHuman {
			t.Fatalf("task %q status at resolve cut = %q, want %q", task.ID, task.Status, TaskStatusWaitingHuman)
		}
	}

	processRunPlanningIntent(t, ctx, svc)

	snapshot, err = svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(after planner resume) error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusRunning {
		t.Fatalf("run lifecycle_status after planner resume = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusRunning)
	}
	taskPage, err = svc.ListRunTasks(ctx, caller, handle.RunID, ListRunTasksRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRunTasks(after planner resume) error = %v", err)
	}
	for _, task := range taskPage.Items {
		if task.Status != TaskStatusReady {
			t.Fatalf("task %q status after planner resume = %q, want %q", task.ID, task.Status, TaskStatusReady)
		}
	}
}

func TestIntegrationGetRunSnapshotAtSeqReturnsHistoricalRunState(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run snapshot should support historical as_of_seq",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	createResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "resume", Kind: CheckpointOptionKindChoice, Label: "Resume"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}
	resolveResult, err := svc.ResolveCheckpoint(ctx, caller, createResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "resume",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	currentSnapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot(current) error = %v", err)
	}
	if currentSnapshot.Run.LifecycleStatus != LifecycleStatusRunning {
		t.Fatalf("current lifecycle_status = %q, want %q", currentSnapshot.Run.LifecycleStatus, LifecycleStatusRunning)
	}
	historicalSnapshot, err := svc.GetRunSnapshotAtSeq(ctx, caller, handle.RunID, createResult.SnapshotSeq)
	if err != nil {
		t.Fatalf("GetRunSnapshotAtSeq(create snapshot) error = %v", err)
	}
	if historicalSnapshot.SnapshotSeq != createResult.SnapshotSeq {
		t.Fatalf("historical snapshot_seq = %d, want %d", historicalSnapshot.SnapshotSeq, createResult.SnapshotSeq)
	}
	if historicalSnapshot.Run.LifecycleStatus != LifecycleStatusWaitingHuman {
		t.Fatalf("historical lifecycle_status = %q, want %q", historicalSnapshot.Run.LifecycleStatus, LifecycleStatusWaitingHuman)
	}
	if resolveResult.SnapshotSeq <= historicalSnapshot.SnapshotSeq {
		t.Fatalf("resolve snapshot_seq = %d, want > %d", resolveResult.SnapshotSeq, historicalSnapshot.SnapshotSeq)
	}
}

func TestIntegrationGetRunSnapshotAtSeqRejectsMissingHistoricalRunProjection(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "missing run projection should not fall back to current state",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "resume", Kind: CheckpointOptionKindChoice, Label: "Resume"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint() error = %v", err)
	}

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	deleteSeq, err := int64FromUint64(checkpointResult.SnapshotSeq, "checkpoint snapshot seq")
	if err != nil {
		t.Fatalf("int64FromUint64(checkpoint snapshot seq) error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM orchestration_projection_snapshots
		WHERE run_id = $1 AND projection_kind = $2 AND seq <= $3
	`, pgRunID, ProjectionKindRun, deleteSeq); err != nil {
		t.Fatalf("delete run projection snapshot: %v", err)
	}

	_, err = svc.GetRunSnapshotAtSeq(ctx, caller, handle.RunID, checkpointResult.SnapshotSeq)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("GetRunSnapshotAtSeq(missing historical projection) error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsUnsupportedTaskState(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "checkpoint should only be allowed on ready tasks",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgTaskID, err := db.ParseUUID(handle.RootTaskID)
	if err != nil {
		t.Fatalf("parse task uuid: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE orchestration_tasks
		SET status = 'running',
		    status_version = status_version + 1,
		    updated_at = now()
		WHERE id = $1
	`, pgTaskID); err != nil {
		t.Fatalf("mark task running: %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause running work?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrTaskCheckpointUnsupported) {
		t.Fatalf("CreateHumanCheckpoint(running task) error = %v, want %v", err, ErrTaskCheckpointUnsupported)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsTimeoutUntilSupported(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "checkpoint timeout should be rejected until runtime support exists",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause with timeout?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
		DefaultAction: &CheckpointDefaultAction{
			Mode:     CheckpointResolutionModeSelectOption,
			OptionID: "ok",
		},
		TimeoutAt: time.Now().Add(time.Minute).UTC(),
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CreateHumanCheckpoint(timeout_at) error = %v, want %v", err, ErrInvalidArgument)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsSecondBarrierOnCapturedTask(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "only one run barrier can remain open",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	_, secondSiblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(second sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "sibling task",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusReady,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   secondSiblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "second sibling task",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusReady,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling.second",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(second sibling) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(first barrier) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         secondSiblingTaskID.String(),
		Question:       "pause run again?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrTaskAlreadyWaitingHuman) {
		t.Fatalf("CreateHumanCheckpoint(second barrier) error = %v, want %v", err, ErrTaskAlreadyWaitingHuman)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsRunBarrierWithWaitingSibling(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should not overlap sibling task checkpoint waits",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "waiting sibling",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusReady,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         siblingTaskID.String(),
		Question:       "need extra details?",
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "details", Kind: CheckpointOptionKindChoice, Label: "Details"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(waiting sibling) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrRunBarrierUnsupported) {
		t.Fatalf("CreateHumanCheckpoint(run barrier with waiting sibling) error = %v, want %v", err, ErrRunBarrierUnsupported)
	}
}

func TestIntegrationCreateHumanCheckpointPausesRunningSibling(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should pause running siblings and restore them to ready",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "running sibling",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "run-barrier-running-sibling",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusDispatching,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}
	_, siblingAttemptID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling attempt) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTaskAttempt(ctx, sqlc.CreateOrchestrationTaskAttemptParams{
		ID:               siblingAttemptID,
		RunID:            pgRunID,
		TaskID:           siblingTaskID,
		AttemptNo:        1,
		Status:           TaskAttemptStatusCreated,
		InputManifestID:  pgtype.UUID{},
		ParkCheckpointID: pgtype.UUID{},
	}); err != nil {
		t.Fatalf("CreateOrchestrationTaskAttempt(sibling) error = %v", err)
	}
	siblingAttempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{"run-barrier-running-sibling"},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt(sibling) error = %v", err)
	}
	runningAttempt, err := svc.StartAttempt(ctx, siblingAttempt.ID, siblingAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(sibling) error = %v", err)
	}

	result, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(running sibling barrier) error = %v", err)
	}

	siblingTask, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(sibling) error = %v", err)
	}
	if siblingTask.Status != TaskStatusWaitingHuman {
		t.Fatalf("sibling task status = %q, want %q", siblingTask.Status, TaskStatusWaitingHuman)
	}
	if siblingTask.WaitingScope != "run" {
		t.Fatalf("sibling waiting_scope = %q, want %q", siblingTask.WaitingScope, "run")
	}
	if !siblingTask.WaitingCheckpointID.Valid || siblingTask.WaitingCheckpointID.String() != result.Checkpoint.ID {
		t.Fatalf("sibling waiting_checkpoint_id = %v, want %q", siblingTask.WaitingCheckpointID, result.Checkpoint.ID)
	}

	storedAttempt, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, siblingAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID(sibling) error = %v", err)
	}
	if storedAttempt.Status != TaskAttemptStatusFailed {
		t.Fatalf("sibling attempt status = %q, want %q", storedAttempt.Status, TaskAttemptStatusFailed)
	}
	if storedAttempt.FailureClass != "run_barrier_preempted" {
		t.Fatalf("sibling attempt failure_class = %q, want %q", storedAttempt.FailureClass, "run_barrier_preempted")
	}
	storedAttemptClaimEpoch, err := uint64FromInt64(storedAttempt.ClaimEpoch, "stored sibling attempt claim epoch")
	if err != nil {
		t.Fatalf("uint64FromInt64(storedAttempt.ClaimEpoch) error = %v", err)
	}
	if storedAttemptClaimEpoch != runningAttempt.ClaimEpoch {
		t.Fatalf("sibling attempt claim_epoch = %d, want %d", storedAttemptClaimEpoch, runningAttempt.ClaimEpoch)
	}

	if _, err := svc.ResolveCheckpoint(ctx, caller, result.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "ok",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	resumedSibling, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(resumed sibling) error = %v", err)
	}
	if resumedSibling.Status != TaskStatusReady {
		t.Fatalf("resumed sibling task status = %q, want %q", resumedSibling.Status, TaskStatusReady)
	}
	if resumedSibling.WaitingScope != "" {
		t.Fatalf("resumed sibling waiting_scope = %q, want empty", resumedSibling.WaitingScope)
	}
}

func TestIntegrationCreateHumanCheckpointPausesDispatchingSibling(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should pause dispatching siblings before execution starts",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "dispatching sibling",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "run-barrier-dispatching-sibling",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusDispatching,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}
	_, siblingAttemptID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling attempt) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTaskAttempt(ctx, sqlc.CreateOrchestrationTaskAttemptParams{
		ID:               siblingAttemptID,
		RunID:            pgRunID,
		TaskID:           siblingTaskID,
		AttemptNo:        1,
		Status:           TaskAttemptStatusCreated,
		InputManifestID:  pgtype.UUID{},
		ParkCheckpointID: pgtype.UUID{},
	}); err != nil {
		t.Fatalf("CreateOrchestrationTaskAttempt(sibling) error = %v", err)
	}

	result, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(dispatching sibling barrier) error = %v", err)
	}

	siblingTask, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(sibling) error = %v", err)
	}
	if siblingTask.Status != TaskStatusWaitingHuman {
		t.Fatalf("sibling task status = %q, want %q", siblingTask.Status, TaskStatusWaitingHuman)
	}
	if siblingTask.WaitingScope != "run" {
		t.Fatalf("sibling waiting_scope = %q, want %q", siblingTask.WaitingScope, "run")
	}
	if !siblingTask.WaitingCheckpointID.Valid || siblingTask.WaitingCheckpointID.String() != result.Checkpoint.ID {
		t.Fatalf("sibling waiting_checkpoint_id = %v, want %q", siblingTask.WaitingCheckpointID, result.Checkpoint.ID)
	}

	siblingAttempt, err := svc.queries.GetOrchestrationTaskAttemptByID(ctx, siblingAttemptID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskAttemptByID(sibling) error = %v", err)
	}
	if siblingAttempt.Status != TaskAttemptStatusFailed {
		t.Fatalf("sibling attempt status = %q, want %q", siblingAttempt.Status, TaskAttemptStatusFailed)
	}
	if siblingAttempt.FailureClass != "run_barrier_paused" {
		t.Fatalf("sibling attempt failure_class = %q, want %q", siblingAttempt.FailureClass, "run_barrier_paused")
	}

	snapshot, err := svc.GetRunSnapshot(ctx, caller, handle.RunID)
	if err != nil {
		t.Fatalf("GetRunSnapshot() error = %v", err)
	}
	if snapshot.Run.LifecycleStatus != LifecycleStatusWaitingHuman {
		t.Fatalf("run lifecycle_status = %q, want %q", snapshot.Run.LifecycleStatus, LifecycleStatusWaitingHuman)
	}

	if _, err := svc.ResolveCheckpoint(ctx, caller, result.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "ok",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	resumedSibling, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(resumed sibling) error = %v", err)
	}
	if resumedSibling.Status != TaskStatusReady {
		t.Fatalf("resumed sibling task status = %q, want %q", resumedSibling.Status, TaskStatusReady)
	}
	if resumedSibling.WaitingScope != "" {
		t.Fatalf("resumed sibling waiting_scope = %q, want empty", resumedSibling.WaitingScope)
	}

	resumedAttempt := dispatchAndClaimAttemptForProfiles(t, ctx, svc, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{"run-barrier-dispatching-sibling"},
		LeaseTTLSeconds: 30,
	}, 3)
	if resumedAttempt.TaskID != siblingTaskID.String() {
		t.Fatalf("resumed sibling attempt task_id = %q, want %q", resumedAttempt.TaskID, siblingTaskID.String())
	}
	startedResumedAttempt, err := svc.StartAttempt(ctx, resumedAttempt.ID, resumedAttempt.ClaimToken)
	if err != nil {
		t.Fatalf("StartAttempt(resumed sibling) error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        startedResumedAttempt.ID,
		ClaimToken:       startedResumedAttempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "dispatching sibling resumed and completed",
		StructuredOutput: map[string]any{"summary": "dispatching sibling done"},
	}); err != nil {
		t.Fatalf("CompleteAttempt(resumed sibling) error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	completedSibling, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(completed sibling) error = %v", err)
	}
	if completedSibling.Status != TaskStatusCompleted {
		t.Fatalf("completed sibling task status = %q, want %q", completedSibling.Status, TaskStatusCompleted)
	}
}

func TestIntegrationCreateHumanCheckpointRequeuesVerifyingSibling(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should requeue verifying siblings and restore them to verifying",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	_, siblingAttemptID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling attempt) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "verifying sibling",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        DefaultRootWorkerProfile,
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(map[string]any{"require_structured_output": true}),
		Status:               TaskStatusVerifying,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTaskAttempt(ctx, sqlc.CreateOrchestrationTaskAttemptParams{
		ID:               siblingAttemptID,
		RunID:            pgRunID,
		TaskID:           siblingTaskID,
		AttemptNo:        1,
		Status:           TaskAttemptStatusCompleted,
		InputManifestID:  pgtype.UUID{},
		ParkCheckpointID: pgtype.UUID{},
	}); err != nil {
		t.Fatalf("CreateOrchestrationTaskAttempt(sibling completed attempt) error = %v", err)
	}
	resultRow, err := svc.queries.CreateOrchestrationTaskResult(ctx, sqlc.CreateOrchestrationTaskResultParams{
		RunID:            pgRunID,
		TaskID:           siblingTaskID,
		AttemptID:        siblingAttemptID,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "sibling result",
		FailureClass:     "",
		RequestReplan:    false,
		ArtifactIntents:  marshalObject(nil),
		StructuredOutput: marshalObject(map[string]any{"summary": "done"}),
	})
	if err != nil {
		t.Fatalf("CreateOrchestrationTaskResult(sibling) error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE orchestration_tasks
		SET latest_result_id = $2, updated_at = now()
		WHERE id = $1
	`, siblingTaskID, resultRow.ID); err != nil {
		t.Fatalf("update sibling latest_result_id: %v", err)
	}
	_, verificationID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(verification) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTaskVerification(ctx, sqlc.CreateOrchestrationTaskVerificationParams{
		ID:              verificationID,
		RunID:           pgRunID,
		TaskID:          siblingTaskID,
		ResultID:        resultRow.ID,
		AttemptNo:       1,
		VerifierProfile: "run-barrier-verifier",
		Status:          TaskVerificationStatusCreated,
	}); err != nil {
		t.Fatalf("CreateOrchestrationTaskVerification(sibling) error = %v", err)
	}

	claimedVerification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{"run-barrier-verifier"},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification(sibling) error = %v", err)
	}
	runningVerification, err := svc.StartVerification(ctx, claimedVerification.ID, claimedVerification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification(sibling) error = %v", err)
	}

	checkpointResult, err := svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHumanCheckpoint(verifying sibling barrier) error = %v", err)
	}

	siblingTask, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(sibling) error = %v", err)
	}
	if siblingTask.Status != TaskStatusWaitingHuman {
		t.Fatalf("sibling task status = %q, want %q", siblingTask.Status, TaskStatusWaitingHuman)
	}
	if siblingTask.WaitingScope != "run" {
		t.Fatalf("sibling waiting_scope = %q, want %q", siblingTask.WaitingScope, "run")
	}
	if !siblingTask.WaitingCheckpointID.Valid || siblingTask.WaitingCheckpointID.String() != checkpointResult.Checkpoint.ID {
		t.Fatalf("sibling waiting_checkpoint_id = %v, want %q", siblingTask.WaitingCheckpointID, checkpointResult.Checkpoint.ID)
	}

	pausedVerification, err := svc.queries.GetOrchestrationTaskVerificationByID(ctx, verificationID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskVerificationByID(sibling) error = %v", err)
	}
	if pausedVerification.Status != TaskVerificationStatusCreated {
		t.Fatalf("paused verification status = %q, want %q", pausedVerification.Status, TaskVerificationStatusCreated)
	}
	pausedVerificationClaimEpoch, err := uint64FromInt64(pausedVerification.ClaimEpoch, "paused verification claim epoch")
	if err != nil {
		t.Fatalf("uint64FromInt64(pausedVerification.ClaimEpoch) error = %v", err)
	}
	if pausedVerificationClaimEpoch != runningVerification.ClaimEpoch {
		t.Fatalf("paused verification claim_epoch = %d, want %d", pausedVerificationClaimEpoch, runningVerification.ClaimEpoch)
	}

	if _, err := svc.ResolveCheckpoint(ctx, caller, checkpointResult.Checkpoint.ID, CheckpointResolution{
		Mode:           CheckpointResolutionModeSelectOption,
		OptionID:       "ok",
		IdempotencyKey: "resolve-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("ResolveCheckpoint() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)

	resumedSibling, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(resumed sibling) error = %v", err)
	}
	if resumedSibling.Status != TaskStatusVerifying {
		t.Fatalf("resumed sibling task status = %q, want %q", resumedSibling.Status, TaskStatusVerifying)
	}
	if resumedSibling.WaitingScope != "" {
		t.Fatalf("resumed sibling waiting_scope = %q, want empty", resumedSibling.WaitingScope)
	}

	reclaimedVerification, err := svc.ClaimNextVerification(ctx, VerificationClaim{
		WorkerID:         "verifier-" + uuid.NewString(),
		ExecutorID:       DefaultVerifierExecutorID,
		VerifierProfiles: []string{"run-barrier-verifier"},
		LeaseTTLSeconds:  30,
	})
	if err != nil {
		t.Fatalf("ClaimNextVerification(after resume) error = %v", err)
	}
	if reclaimedVerification.ID != verificationID.String() {
		t.Fatalf("reclaimed verification id = %q, want %q", reclaimedVerification.ID, verificationID.String())
	}
	restartedVerification, err := svc.StartVerification(ctx, reclaimedVerification.ID, reclaimedVerification.ClaimToken)
	if err != nil {
		t.Fatalf("StartVerification(after resume) error = %v", err)
	}
	if _, err := svc.CompleteVerification(ctx, VerificationCompletion{
		VerificationID: restartedVerification.ID,
		ClaimToken:     restartedVerification.ClaimToken,
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "verifying sibling resumed and completed",
	}); err != nil {
		t.Fatalf("CompleteVerification(after resume) error = %v", err)
	}

	completedSibling, err := svc.queries.GetOrchestrationTaskByID(ctx, siblingTaskID)
	if err != nil {
		t.Fatalf("GetOrchestrationTaskByID(completed sibling) error = %v", err)
	}
	if completedSibling.Status != TaskStatusCompleted {
		t.Fatalf("completed sibling task status = %q, want %q", completedSibling.Status, TaskStatusCompleted)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsDispatchingSiblingWithoutAttempt(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should reject dispatching sibling without attempt",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "dispatching sibling without attempt",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "run-barrier-dispatching-missing-attempt",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusDispatching,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrRunBarrierUnsupported) {
		t.Fatalf("CreateHumanCheckpoint(dispatching sibling without attempt) error = %v, want %v", err, ErrRunBarrierUnsupported)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsRunningSiblingWithoutAttempt(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should reject running sibling without attempt",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "running sibling without attempt",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        "run-barrier-running-missing-attempt",
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusRunning,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrRunBarrierUnsupported) {
		t.Fatalf("CreateHumanCheckpoint(running sibling without attempt) error = %v, want %v", err, ErrRunBarrierUnsupported)
	}
}

func TestIntegrationCreateHumanCheckpointRejectsVerifyingSiblingWithoutVerification(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should reject verifying sibling without verification",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	pgRunID, err := db.ParseUUID(handle.RunID)
	if err != nil {
		t.Fatalf("parse run uuid: %v", err)
	}
	_, siblingTaskID, err := newPGUUID()
	if err != nil {
		t.Fatalf("newPGUUID(sibling) error = %v", err)
	}
	if _, err := svc.queries.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   siblingTaskID,
		RunID:                pgRunID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "child",
		Goal:                 "verifying sibling without verification",
		Inputs:               marshalObject(map[string]any{"kind": "sibling"}),
		PlannerEpoch:         1,
		WorkerProfile:        DefaultRootWorkerProfile,
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(map[string]any{"require_structured_output": true}),
		Status:               TaskStatusVerifying,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.sibling",
	}); err != nil {
		t.Fatalf("CreateOrchestrationTask(sibling) error = %v", err)
	}

	_, err = svc.CreateHumanCheckpoint(ctx, caller, CreateHumanCheckpointRequest{
		RunID:          handle.RunID,
		TaskID:         handle.RootTaskID,
		Question:       "pause run?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrRunBarrierUnsupported) {
		t.Fatalf("CreateHumanCheckpoint(verifying sibling without verification) error = %v, want %v", err, ErrRunBarrierUnsupported)
	}
}
