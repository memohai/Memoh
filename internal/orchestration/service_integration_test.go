package orchestration

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
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

func TestIntegrationTenantScopedAuthorizationRejectsSameSubjectAcrossTenants(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	subject := "shared-subject-" + uuid.NewString()
	caller := ControlIdentity{
		TenantID: "tenant-a-" + uuid.NewString(),
		Subject:  subject,
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
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, otherTenantCaller.TenantID, otherTenantCaller.Subject)

	if _, err := svc.GetRunSnapshot(ctx, otherTenantCaller, handle.RunID); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("GetRunSnapshot(cross-tenant same-subject) error = %v, want %v", err, ErrAccessDenied)
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
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("ResolveCheckpoint(cross-tenant same-subject) error = %v, want %v", err, ErrAccessDenied)
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
		found = true
	}
	if !found {
		t.Fatal("ListRunEvents() missing run.event.artifact.committed")
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

func TestIntegrationCreateHumanCheckpointRejectsSecondOpenRunBarrier(t *testing.T) {
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
		TaskID:         siblingTaskID.String(),
		Question:       "pause run again?",
		BlocksRun:      true,
		IdempotencyKey: "checkpoint-" + uuid.NewString(),
		Options: []CheckpointOption{
			{ID: "ok", Kind: CheckpointOptionKindChoice, Label: "OK"},
		},
	})
	if !errors.Is(err, ErrRunBarrierAlreadyOpen) {
		t.Fatalf("CreateHumanCheckpoint(second barrier) error = %v, want %v", err, ErrRunBarrierAlreadyOpen)
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

func TestIntegrationCreateHumanCheckpointRejectsRunBarrierWithActiveSibling(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-" + uuid.NewString(),
		Subject:  "subject-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run barrier should not capture active siblings without park support",
		IdempotencyKey: "start-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
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
		WorkerProfile:        "",
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
		t.Fatalf("CreateHumanCheckpoint(active sibling barrier) error = %v, want %v", err, ErrRunBarrierUnsupported)
	}
}
