package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	pool               *pgxpool.Pool
	queries            *sqlc.Queries
	logger             *slog.Logger
	attemptAssignments AttemptAssignmentPublisher
}

func NewService(log *slog.Logger, pool *pgxpool.Pool, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		pool:               pool,
		queries:            queries,
		logger:             log.With(slog.String("service", "orchestration")),
		attemptAssignments: noopAttemptAssignmentPublisher{},
	}
}

func (s *Service) StartRun(ctx context.Context, caller ControlIdentity, req StartRunRequest) (RunHandle, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return RunHandle{}, err
	}
	normalizedGoal := strings.TrimSpace(req.Goal)
	normalizedIdempotencyKey := normalizeIdempotencyKey(req.IdempotencyKey)
	if normalizedGoal == "" {
		return RunHandle{}, fmt.Errorf("%w: goal is required", ErrInvalidArgument)
	}
	if normalizedIdempotencyKey == "" {
		return RunHandle{}, fmt.Errorf("%w: idempotency_key is required", ErrInvalidArgument)
	}

	hash, err := startRunRequestHash(req)
	if err != nil {
		return RunHandle{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RunHandle{}, fmt.Errorf("begin orchestration start tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	record, replay, err := ensureIdempotencyRecord(ctx, qtx, caller, methodStartRun, "start_run", normalizedIdempotencyKey, hash)
	if err != nil {
		return RunHandle{}, err
	}
	if replay {
		handle, err := decodeRunHandle(record.ResponsePayload)
		if err != nil {
			return RunHandle{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return RunHandle{}, fmt.Errorf("commit orchestration idempotent start tx: %w", err)
		}
		return handle, nil
	}

	runID, runUUID, err := newPGUUID()
	if err != nil {
		return RunHandle{}, err
	}
	rootTaskID, rootTaskUUID, err := newPGUUID()
	if err != nil {
		return RunHandle{}, err
	}

	runRow, err := qtx.CreateOrchestrationRun(ctx, sqlc.CreateOrchestrationRunParams{
		ID:                     runUUID,
		TenantID:               caller.TenantID,
		OwnerSubject:           caller.Subject,
		LifecycleStatus:        LifecycleStatusCreated,
		PlanningStatus:         PlanningStatusActive,
		StatusVersion:          1,
		PlannerEpoch:           1,
		LastEventSeq:           0,
		RootTaskID:             rootTaskUUID,
		Goal:                   normalizedGoal,
		Input:                  marshalObject(req.Input),
		OutputSchema:           marshalObject(req.OutputSchema),
		RequestedControlPolicy: marshalObject(req.RequestedControlPolicy),
		ControlPolicy:          marshalJSON(buildPhase1ControlPolicy(caller)),
		SourceMetadata:         marshalObject(req.SourceMetadata),
		Policies:               marshalObject(req.Policies),
		CreatedBy:              caller.Subject,
		TerminalReason:         "",
	})
	if err != nil {
		return RunHandle{}, fmt.Errorf("create orchestration run: %w", err)
	}

	if _, err := s.appendEvent(ctx, qtx, runUUID, eventSpec{
		TaskID:           pgtype.UUID{},
		CheckpointID:     pgtype.UUID{},
		AggregateType:    "run",
		AggregateID:      runUUID,
		AggregateVersion: runRow.StatusVersion,
		Type:             "run.event.created",
		IdempotencyKey:   normalizedIdempotencyKey,
		Payload: map[string]any{
			"run_id":          runID,
			"root_task_id":    rootTaskID,
			"planner_epoch":   runRow.PlannerEpoch,
			"previous_status": "",
			"new_status":      runRow.LifecycleStatus,
		},
	}); err != nil {
		return RunHandle{}, err
	}

	taskRow, err := qtx.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
		ID:                   rootTaskUUID,
		RunID:                runUUID,
		DecomposedFromTaskID: pgtype.UUID{},
		Kind:                 "root",
		Goal:                 normalizedGoal,
		Inputs:               marshalObject(req.Input),
		PlannerEpoch:         1,
		WorkerProfile:        DefaultRootWorkerProfile,
		Priority:             0,
		RetryPolicy:          marshalObject(nil),
		VerificationPolicy:   marshalObject(nil),
		Status:               TaskStatusCreated,
		StatusVersion:        1,
		WaitingScope:         "",
		BlockedReason:        "",
		TerminalReason:       "",
		BlackboardScope:      "run.root",
	})
	if err != nil {
		return RunHandle{}, fmt.Errorf("create orchestration root task: %w", err)
	}

	if _, err := s.appendEvent(ctx, qtx, runUUID, eventSpec{
		TaskID:           rootTaskUUID,
		CheckpointID:     pgtype.UUID{},
		AggregateType:    "task",
		AggregateID:      rootTaskUUID,
		AggregateVersion: taskRow.StatusVersion,
		Type:             "run.event.task.created",
		IdempotencyKey:   normalizedIdempotencyKey,
		Payload: map[string]any{
			"task_id":          rootTaskID,
			"run_id":           runID,
			"previous_status":  "",
			"new_status":       taskRow.Status,
			"planner_epoch":    taskRow.PlannerEpoch,
			"worker_profile":   taskRow.WorkerProfile,
			"blackboard_scope": taskRow.BlackboardScope,
		},
	}); err != nil {
		return RunHandle{}, err
	}

	planningIntentID, planningIntentUUID, err := newPGUUID()
	if err != nil {
		return RunHandle{}, err
	}
	if _, err := qtx.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               planningIntentUUID,
		RunID:            runUUID,
		TaskID:           rootTaskUUID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindStartRun,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: runRow.PlannerEpoch,
		Payload: marshalJSON(map[string]any{
			"run_id":     runID,
			"task_id":    rootTaskID,
			"reason":     "start_run",
			"goal":       normalizedGoal,
			"created_by": caller.Subject,
		}),
	}); err != nil {
		return RunHandle{}, fmt.Errorf("create orchestration planning intent: %w", err)
	}
	planningEvent, err := s.appendEvent(ctx, qtx, runUUID, eventSpec{
		TaskID:           rootTaskUUID,
		CheckpointID:     pgtype.UUID{},
		AggregateType:    "planning_intent",
		AggregateID:      planningIntentUUID,
		AggregateVersion: 1,
		Type:             "run.event.planning_intent.enqueued",
		IdempotencyKey:   normalizedIdempotencyKey,
		Payload: map[string]any{
			"planning_intent_id": planningIntentID,
			"run_id":             runID,
			"task_id":            rootTaskID,
			"kind":               PlanningIntentKindStartRun,
			"status":             PlanningIntentStatusPending,
		},
	})
	if err != nil {
		return RunHandle{}, err
	}

	snapshotSeq, err := uint64FromInt64(planningEvent.Seq, "run event seq")
	if err != nil {
		return RunHandle{}, err
	}
	handle := RunHandle{
		RunID:       runID,
		RootTaskID:  rootTaskID,
		SnapshotSeq: snapshotSeq,
	}
	if _, err := qtx.CompleteOrchestrationIdempotencyRecord(ctx, sqlc.CompleteOrchestrationIdempotencyRecordParams{
		ResponsePayload: marshalJSON(handle),
		TenantID:        caller.TenantID,
		CallerSubject:   caller.Subject,
		Method:          methodStartRun,
		TargetID:        "start_run",
		IdempotencyKey:  normalizedIdempotencyKey,
	}); err != nil {
		return RunHandle{}, fmt.Errorf("complete orchestration start idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RunHandle{}, fmt.Errorf("commit orchestration start tx: %w", err)
	}
	return handle, nil
}

func (s *Service) GetRunSnapshot(ctx context.Context, caller ControlIdentity, runID string) (*RunSnapshot, error) {
	return s.GetRunSnapshotAtSeq(ctx, caller, runID, 0)
}

func (s *Service) GetRunSnapshotAtSeq(ctx context.Context, caller ControlIdentity, runID string, asOfSeq uint64) (*RunSnapshot, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	row, err := s.getAuthorizedRun(ctx, caller, runID)
	if err != nil {
		return nil, err
	}
	lastEventSeq, err := uint64FromInt64(row.LastEventSeq, "run last_event_seq")
	if err != nil {
		return nil, err
	}
	resolvedSeq := lastEventSeq
	if asOfSeq != 0 {
		if asOfSeq > lastEventSeq {
			return nil, fmt.Errorf("%w: as_of_seq exceeds current snapshot", ErrInvalidArgument)
		}
		resolvedSeq = asOfSeq
	}
	runSnapshot, snapshotSeq, err := s.loadRunSnapshot(ctx, row.ID, resolvedSeq)
	if err != nil {
		return nil, err
	}
	if snapshotSeq == 0 {
		if asOfSeq != 0 {
			return nil, fmt.Errorf("%w: run snapshot unavailable at as_of_seq", ErrInvalidArgument)
		}
		runSnapshot = toRun(row)
		snapshotSeq = lastEventSeq
	}
	return &RunSnapshot{
		Run:         runSnapshot,
		SnapshotSeq: snapshotSeq,
	}, nil
}

func (s *Service) ListRunTasks(ctx context.Context, caller ControlIdentity, runID string, req ListRunTasksRequest) (*TaskPage, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	row, err := s.getAuthorizedRun(ctx, caller, runID)
	if err != nil {
		return nil, err
	}
	currentSeq, err := uint64FromInt64(row.LastEventSeq, "run last_event_seq")
	if err != nil {
		return nil, err
	}
	asOfSeq, err := resolvePageAsOfSeq(currentSeq, req.AsOfSeq, req.After)
	if err != nil {
		return nil, err
	}
	items, snapshotSeq, err := s.loadTaskSnapshot(ctx, row.ID, asOfSeq)
	if err != nil {
		return nil, err
	}
	items = filterTasks(items, req.Status)
	pageItems, nextAfter, err := paginateTasks(items, req.After, normalizedListLimit(req.Limit), snapshotSeq, filterHash(req.Status))
	if err != nil {
		return nil, err
	}
	return &TaskPage{
		Items:       pageItems,
		NextAfter:   nextAfter,
		SnapshotSeq: snapshotSeq,
	}, nil
}

func (s *Service) ListRunCheckpoints(ctx context.Context, caller ControlIdentity, runID string, req ListRunCheckpointsRequest) (*HumanCheckpointPage, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	row, err := s.getAuthorizedRun(ctx, caller, runID)
	if err != nil {
		return nil, err
	}
	currentSeq, err := uint64FromInt64(row.LastEventSeq, "run last_event_seq")
	if err != nil {
		return nil, err
	}
	asOfSeq, err := resolvePageAsOfSeq(currentSeq, req.AsOfSeq, req.After)
	if err != nil {
		return nil, err
	}
	items, snapshotSeq, err := s.loadCheckpointSnapshot(ctx, row.ID, asOfSeq)
	if err != nil {
		return nil, err
	}
	items = filterCheckpoints(items, req.Status)
	pageItems, nextAfter, err := paginateCheckpoints(items, req.After, normalizedListLimit(req.Limit), snapshotSeq, filterHash(req.Status))
	if err != nil {
		return nil, err
	}
	return &HumanCheckpointPage{
		Items:       pageItems,
		NextAfter:   nextAfter,
		SnapshotSeq: snapshotSeq,
	}, nil
}

func (s *Service) ListRunArtifacts(ctx context.Context, caller ControlIdentity, runID string, req ListRunArtifactsRequest) (*ArtifactPage, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	row, err := s.getAuthorizedRun(ctx, caller, runID)
	if err != nil {
		return nil, err
	}
	currentSeq, err := uint64FromInt64(row.LastEventSeq, "run last_event_seq")
	if err != nil {
		return nil, err
	}
	asOfSeq, err := resolvePageAsOfSeq(currentSeq, req.AsOfSeq, req.After)
	if err != nil {
		return nil, err
	}
	items, snapshotSeq, err := s.loadArtifactSnapshot(ctx, row.ID, asOfSeq)
	if err != nil {
		return nil, err
	}
	normalizedTaskID, err := normalizeOptionalUUID(req.TaskID, "task id")
	if err != nil {
		return nil, err
	}
	items = filterArtifacts(items, normalizedTaskID, req.Kind)
	pageItems, nextAfter, err := paginateArtifacts(items, req.After, normalizedListLimit(req.Limit), snapshotSeq, filterHash([]string{normalizedTaskID}, req.Kind))
	if err != nil {
		return nil, err
	}
	return &ArtifactPage{
		Items:       pageItems,
		NextAfter:   nextAfter,
		SnapshotSeq: snapshotSeq,
	}, nil
}

func (s *Service) ListRunEvents(ctx context.Context, caller ControlIdentity, runID string, req ListRunEventsRequest) (*RunEventPage, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	row, err := s.getAuthorizedRun(ctx, caller, runID)
	if err != nil {
		return nil, err
	}
	pgRunID, err := db.ParseUUID(runID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run id", ErrInvalidArgument)
	}
	currentSeq, err := uint64FromInt64(row.LastEventSeq, "run last_event_seq")
	if err != nil {
		return nil, err
	}
	untilSeq, err := resolveEventUntilSeq(currentSeq, req.AfterSeq, req.UntilSeq)
	if err != nil {
		return nil, err
	}
	afterSeq, err := int64FromUint64(req.AfterSeq, "after_seq")
	if err != nil {
		return nil, err
	}
	untilSeqInt64, err := int64FromUint64(untilSeq, "until_seq")
	if err != nil {
		return nil, err
	}
	limitCount, err := int32FromInt(normalizedEventLimit(req.Limit), "event limit")
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListOrchestrationRunEvents(ctx, sqlc.ListOrchestrationRunEventsParams{
		RunID:      pgRunID,
		AfterSeq:   afterSeq,
		UntilSeq:   untilSeqInt64,
		LimitCount: limitCount,
	})
	if err != nil {
		return nil, fmt.Errorf("list orchestration events: %w", err)
	}
	events := make([]RunEvent, 0, len(rows))
	for _, item := range rows {
		events = append(events, toRunEvent(item))
	}
	return &RunEventPage{
		Items:    events,
		UntilSeq: untilSeq,
	}, nil
}

func (s *Service) ResolveCheckpoint(ctx context.Context, caller ControlIdentity, checkpointID string, input CheckpointResolution) (*ResolveCheckpointResult, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	normalizedIdempotencyKey := normalizeIdempotencyKey(input.IdempotencyKey)
	if normalizedIdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidArgument)
	}
	pgCheckpointID, err := db.ParseUUID(checkpointID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid checkpoint id", ErrInvalidArgument)
	}
	normalizedCheckpointID := pgCheckpointID.String()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin resolve checkpoint tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	existing, existingFound, err := getIdempotencyRecord(ctx, qtx, caller, methodResolveCheckpoint, normalizedCheckpointID, normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}

	lockedCheckpoint, err := qtx.GetOrchestrationHumanCheckpointByIDForUpdate(ctx, pgCheckpointID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCheckpointNotFound
		}
		return nil, fmt.Errorf("lock checkpoint: %w", err)
	}
	lockedRun, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, lockedCheckpoint.RunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock checkpoint run: %w", err)
	}
	if err := authorizeRun(caller, lockedRun); err != nil {
		return nil, ErrCheckpointNotFound
	}

	normalized, hash, err := normalizeCheckpointResolution(lockedCheckpoint, input)
	if err != nil {
		return nil, err
	}
	if existingFound {
		if existing.RequestHash != hash {
			return nil, ErrIdempotencyConflict
		}
		if existing.State != "completed" {
			return nil, ErrIdempotencyIncomplete
		}
		result, err := decodeResolveCheckpointResult(existing.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed resolve checkpoint tx: %w", err)
		}
		return &result, nil
	}

	record, replay, err := ensureIdempotencyRecord(ctx, qtx, caller, methodResolveCheckpoint, normalizedCheckpointID, normalizedIdempotencyKey, hash)
	if err != nil {
		return nil, err
	}
	if replay {
		result, err := decodeResolveCheckpointResult(record.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed resolve checkpoint tx: %w", err)
		}
		return &result, nil
	}

	if !runAcceptsExternalMutations(lockedRun.LifecycleStatus) {
		return nil, ErrRunImmutable
	}
	if lockedCheckpoint.Status != CheckpointStatusOpen {
		return nil, ErrCheckpointNotOpen
	}

	lockedTask, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, lockedCheckpoint.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock checkpoint task: %w", err)
	}

	resolvedCheckpoint, err := qtx.ResolveOrchestrationHumanCheckpoint(ctx, sqlc.ResolveOrchestrationHumanCheckpointParams{
		ID:                    lockedCheckpoint.ID,
		ResolvedBy:            caller.Subject,
		ResolvedMode:          normalized.Mode,
		ResolvedOptionID:      normalized.OptionID,
		ResolvedFreeformInput: normalized.FreeformInput,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve checkpoint: %w", err)
	}

	_, err = s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
		TaskID:           lockedCheckpoint.TaskID,
		CheckpointID:     lockedCheckpoint.ID,
		AggregateType:    "checkpoint",
		AggregateID:      lockedCheckpoint.ID,
		AggregateVersion: resolvedCheckpoint.StatusVersion,
		Type:             "run.event.hitl.resolved",
		IdempotencyKey:   normalizedIdempotencyKey,
		Payload: map[string]any{
			"checkpoint_id":           resolvedCheckpoint.ID.String(),
			"task_id":                 resolvedCheckpoint.TaskID.String(),
			"previous_status":         lockedCheckpoint.Status,
			"new_status":              resolvedCheckpoint.Status,
			"status":                  resolvedCheckpoint.Status,
			"blocks_run":              resolvedCheckpoint.BlocksRun,
			"resolved_by":             caller.Subject,
			"resolved_mode":           normalized.Mode,
			"resolved_option_id":      normalized.OptionID,
			"resolved_freeform_input": normalized.FreeformInput,
			"resolution_metadata":     normalized.Metadata,
		},
	})
	if err != nil {
		return nil, err
	}

	_, planningIntentUUID, err := newPGUUID()
	if err != nil {
		return nil, err
	}
	planningIntent, err := qtx.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               planningIntentUUID,
		RunID:            lockedRun.ID,
		TaskID:           lockedTask.ID,
		CheckpointID:     resolvedCheckpoint.ID,
		Kind:             PlanningIntentKindCheckpointResume,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: lockedRun.PlannerEpoch,
		Payload: marshalJSON(map[string]any{
			"run_id":              lockedRun.ID.String(),
			"task_id":             lockedTask.ID.String(),
			"checkpoint_id":       resolvedCheckpoint.ID.String(),
			"checkpoint_blocking": resolvedCheckpoint.BlocksRun,
			"resolved_by":         caller.Subject,
			"resolved_mode":       normalized.Mode,
			"resolution_metadata": normalized.Metadata,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("create checkpoint resume planning intent: %w", err)
	}
	_, err = s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
		TaskID:           lockedTask.ID,
		CheckpointID:     resolvedCheckpoint.ID,
		AggregateType:    "planning_intent",
		AggregateID:      planningIntent.ID,
		AggregateVersion: 1,
		Type:             "run.event.planning_intent.enqueued",
		IdempotencyKey:   normalizedIdempotencyKey,
		Payload: map[string]any{
			"planning_intent_id": planningIntent.ID.String(),
			"run_id":             lockedRun.ID.String(),
			"task_id":            lockedTask.ID.String(),
			"checkpoint_id":      resolvedCheckpoint.ID.String(),
			"kind":               planningIntent.Kind,
			"status":             planningIntent.Status,
		},
	})
	if err != nil {
		return nil, err
	}
	if err := s.syncRunPlanningStatus(ctx, qtx, lockedRun.ID); err != nil {
		return nil, err
	}
	lockedRun, err = qtx.GetOrchestrationRunByIDForUpdate(ctx, lockedRun.ID)
	if err != nil {
		return nil, fmt.Errorf("lock run after checkpoint resolve: %w", err)
	}

	snapshotSeq, err := uint64FromInt64(lockedRun.LastEventSeq, "checkpoint resolve event seq")
	if err != nil {
		return nil, err
	}
	result := ResolveCheckpointResult{
		CheckpointID: normalizedCheckpointID,
		SnapshotSeq:  snapshotSeq,
	}
	if _, err := qtx.CompleteOrchestrationIdempotencyRecord(ctx, sqlc.CompleteOrchestrationIdempotencyRecordParams{
		ResponsePayload: marshalJSON(result),
		TenantID:        caller.TenantID,
		CallerSubject:   caller.Subject,
		Method:          methodResolveCheckpoint,
		TargetID:        normalizedCheckpointID,
		IdempotencyKey:  normalizedIdempotencyKey,
	}); err != nil {
		return nil, fmt.Errorf("complete resolve checkpoint idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit resolve checkpoint tx: %w", err)
	}
	return &result, nil
}

func (s *Service) CreateHumanCheckpoint(ctx context.Context, caller ControlIdentity, req CreateHumanCheckpointRequest) (*CreateHumanCheckpointResult, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	normalizedIdempotencyKey := normalizeIdempotencyKey(req.IdempotencyKey)
	if strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.TaskID) == "" || strings.TrimSpace(req.Question) == "" {
		return nil, fmt.Errorf("%w: run_id, task_id, and question are required", ErrInvalidArgument)
	}
	if len(req.Options) == 0 {
		return nil, fmt.Errorf("%w: at least one checkpoint option is required", ErrInvalidArgument)
	}
	if normalizedIdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidArgument)
	}
	if !req.TimeoutAt.IsZero() {
		return nil, fmt.Errorf("%w: timeout_at is not supported by current runtime", ErrInvalidArgument)
	}

	pgRunID, err := db.ParseUUID(req.RunID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run id", ErrInvalidArgument)
	}
	pgTaskID, err := db.ParseUUID(req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid task id", ErrInvalidArgument)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin create checkpoint tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	lockedRun, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, pgRunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for checkpoint create: %w", err)
	}
	if err := authorizeRun(caller, lockedRun); err != nil {
		return nil, ErrRunNotFound
	}
	lockedTask, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, pgTaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock task for checkpoint create: %w", err)
	}
	if lockedTask.RunID != lockedRun.ID {
		return nil, ErrTaskNotFound
	}
	if !runAcceptsExternalMutations(lockedRun.LifecycleStatus) && lockedRun.LifecycleStatus != LifecycleStatusWaitingHuman {
		return nil, ErrRunImmutable
	}

	normalizedOptions, normalizedDefaultAction, err := validateCheckpointDefinition(req.Options, req.DefaultAction, req.TimeoutAt)
	if err != nil {
		return nil, err
	}
	normalizedResumePolicy, err := normalizeCheckpointResumePolicy(req.ResumePolicy)
	if err != nil {
		return nil, err
	}
	requestHash, err := createHumanCheckpointRequestHash(req, normalizedOptions, normalizedDefaultAction, normalizedResumePolicy, normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}

	existing, existingFound, err := getIdempotencyRecord(ctx, qtx, caller, methodCreateHumanCheckpoint, pgTaskID.String(), normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existingFound {
		if existing.RequestHash != requestHash {
			return nil, ErrIdempotencyConflict
		}
		if existing.State != "completed" {
			return nil, ErrIdempotencyIncomplete
		}
		result, err := decodeCreateHumanCheckpointResult(existing.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed create checkpoint tx: %w", err)
		}
		return &result, nil
	}

	if taskSuperseded(lockedTask) {
		return nil, ErrTaskImmutable
	}
	if lockedTask.Status == TaskStatusWaitingHuman || lockedTask.WaitingCheckpointID.Valid {
		return nil, ErrTaskAlreadyWaitingHuman
	}
	if !taskAcceptsHumanCheckpoint(lockedTask.Status) {
		return nil, ErrTaskImmutable
	}
	if !taskSupportsHumanCheckpoint(lockedTask.Status) {
		return nil, ErrTaskCheckpointUnsupported
	}

	record, replay, err := ensureIdempotencyRecord(ctx, qtx, caller, methodCreateHumanCheckpoint, pgTaskID.String(), normalizedIdempotencyKey, requestHash)
	if err != nil {
		return nil, err
	}
	if replay {
		result, err := decodeCreateHumanCheckpointResult(record.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed create checkpoint tx: %w", err)
		}
		return &result, nil
	}

	if req.BlocksRun {
		openBlockingCount, err := qtx.CountOpenRunBlockingCheckpointsByRun(ctx, pgRunID)
		if err != nil {
			return nil, fmt.Errorf("count open run blocking checkpoints: %w", err)
		}
		if openBlockingCount > 0 {
			return nil, ErrRunBarrierAlreadyOpen
		}
		siblingTasks, err := qtx.ListCurrentOrchestrationTasksByRun(ctx, pgRunID)
		if err != nil {
			return nil, fmt.Errorf("list sibling tasks for run barrier preflight: %w", err)
		}
		for _, sibling := range siblingTasks {
			if sibling.ID == lockedTask.ID {
				continue
			}
			if taskBlocksRunBarrier(sibling) {
				return nil, ErrRunBarrierUnsupported
			}
		}
	}

	checkpointID, checkpointUUID, err := newPGUUID()
	if err != nil {
		return nil, err
	}
	checkpointRow, err := qtx.CreateOrchestrationHumanCheckpoint(ctx, sqlc.CreateOrchestrationHumanCheckpointParams{
		ID:            checkpointUUID,
		RunID:         pgRunID,
		TaskID:        pgTaskID,
		BlocksRun:     req.BlocksRun,
		PlannerEpoch:  lockedRun.PlannerEpoch,
		Status:        CheckpointStatusOpen,
		StatusVersion: 1,
		Question:      strings.TrimSpace(req.Question),
		Options:       marshalJSON(normalizedOptions),
		DefaultAction: marshalJSON(normalizedDefaultAction),
		ResumePolicy:  marshalJSON(normalizedResumePolicy),
		TimeoutAt:     timeToPg(req.TimeoutAt),
		Metadata:      marshalObject(req.Metadata),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_orchestration_human_checkpoints_open_run_barrier_unique" {
			return nil, ErrRunBarrierAlreadyOpen
		}
		return nil, fmt.Errorf("create human checkpoint: %w", err)
	}

	lastEvent, err := s.appendEvent(ctx, qtx, pgRunID, eventSpec{
		TaskID:           lockedTask.ID,
		CheckpointID:     checkpointUUID,
		AggregateType:    "checkpoint",
		AggregateID:      checkpointUUID,
		AggregateVersion: checkpointRow.StatusVersion,
		Type:             "run.event.hitl.requested",
		Payload: map[string]any{
			"checkpoint_id":  checkpointID,
			"task_id":        checkpointRow.TaskID.String(),
			"status":         checkpointRow.Status,
			"blocks_run":     checkpointRow.BlocksRun,
			"question":       checkpointRow.Question,
			"options":        normalizedOptions,
			"default_action": normalizedDefaultAction,
			"resume_policy":  normalizedResumePolicy,
			"timeout_at":     timeForJSON(db.TimeFromPg(checkpointRow.TimeoutAt)),
			"metadata":       normalizeObject(req.Metadata),
		},
	})
	if err != nil {
		return nil, err
	}

	waitingTask, err := qtx.MarkOrchestrationTaskWaitingHuman(ctx, sqlc.MarkOrchestrationTaskWaitingHumanParams{
		ID:                  lockedTask.ID,
		WaitingCheckpointID: checkpointUUID,
		WaitingScope:        "task",
	})
	if err != nil {
		return nil, fmt.Errorf("mark task waiting on checkpoint: %w", err)
	}

	lastEvent, err = s.appendEvent(ctx, qtx, pgRunID, eventSpec{
		TaskID:           waitingTask.ID,
		CheckpointID:     checkpointUUID,
		AggregateType:    "task",
		AggregateID:      waitingTask.ID,
		AggregateVersion: waitingTask.StatusVersion,
		Type:             "run.event.task.waiting_human",
		Payload: map[string]any{
			"task_id":               waitingTask.ID.String(),
			"previous_status":       lockedTask.Status,
			"new_status":            waitingTask.Status,
			"waiting_scope":         waitingTask.WaitingScope,
			"waiting_checkpoint_id": checkpointID,
		},
	})
	if err != nil {
		return nil, err
	}

	if req.BlocksRun && lockedRun.LifecycleStatus != LifecycleStatusWaitingHuman {
		siblingTasks, err := qtx.ListCurrentOrchestrationTasksByRun(ctx, pgRunID)
		if err != nil {
			return nil, fmt.Errorf("list sibling tasks for run barrier: %w", err)
		}
		runAttempts, err := qtx.ListCurrentOrchestrationTaskAttemptsByRun(ctx, pgRunID)
		if err != nil {
			return nil, fmt.Errorf("list run attempts for run barrier: %w", err)
		}
		runVerifications, err := qtx.ListCurrentOrchestrationTaskVerificationsByRun(ctx, pgRunID)
		if err != nil {
			return nil, fmt.Errorf("list run verifications for run barrier: %w", err)
		}
		for _, sibling := range siblingTasks {
			if sibling.ID == lockedTask.ID {
				continue
			}
			if !taskPauseableByRunBarrier(sibling) {
				if taskBlocksRunBarrier(sibling) {
					return nil, ErrRunBarrierUnsupported
				}
				continue
			}
			lockedSibling, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, sibling.ID)
			if err != nil {
				return nil, fmt.Errorf("lock sibling task for run barrier: %w", err)
			}
			if !taskPauseableByRunBarrier(lockedSibling) {
				if taskBlocksRunBarrier(lockedSibling) {
					return nil, ErrRunBarrierUnsupported
				}
				continue
			}
			if err := s.pauseRunBarrierSiblingWork(ctx, qtx, pgRunID, checkpointUUID, lockedSibling, runAttempts, runVerifications); err != nil {
				return nil, err
			}
			waitingSibling, err := qtx.MarkOrchestrationTaskWaitingHuman(ctx, sqlc.MarkOrchestrationTaskWaitingHumanParams{
				ID:                  lockedSibling.ID,
				WaitingCheckpointID: checkpointUUID,
				WaitingScope:        "run",
			})
			if err != nil {
				return nil, fmt.Errorf("mark sibling task waiting on run checkpoint: %w", err)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, pgRunID, eventSpec{
				TaskID:           waitingSibling.ID,
				CheckpointID:     checkpointUUID,
				AggregateType:    "task",
				AggregateID:      waitingSibling.ID,
				AggregateVersion: waitingSibling.StatusVersion,
				Type:             "run.event.task.waiting_human",
				Payload: map[string]any{
					"task_id":               waitingSibling.ID.String(),
					"previous_status":       lockedSibling.Status,
					"new_status":            waitingSibling.Status,
					"waiting_scope":         waitingSibling.WaitingScope,
					"waiting_checkpoint_id": checkpointID,
				},
			})
			if err != nil {
				return nil, err
			}
		}
		waitingRun, err := qtx.MarkOrchestrationRunWaitingHuman(ctx, lockedRun.ID)
		if err != nil {
			return nil, fmt.Errorf("mark run waiting_human: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, pgRunID, eventSpec{
			TaskID:           pgtype.UUID{},
			CheckpointID:     checkpointUUID,
			AggregateType:    "run",
			AggregateID:      waitingRun.ID,
			AggregateVersion: waitingRun.StatusVersion,
			Type:             "run.event.waiting_human",
			Payload: map[string]any{
				"run_id":          waitingRun.ID.String(),
				"checkpoint_id":   checkpointID,
				"blocks_run":      true,
				"previous_status": lockedRun.LifecycleStatus,
				"new_status":      waitingRun.LifecycleStatus,
			},
		})
		if err != nil {
			return nil, err
		}
	}

	snapshotSeq, err := uint64FromInt64(lastEvent.Seq, "checkpoint create event seq")
	if err != nil {
		return nil, err
	}
	result := CreateHumanCheckpointResult{
		Checkpoint:  toHumanCheckpoint(checkpointRow),
		SnapshotSeq: snapshotSeq,
	}
	if _, err := qtx.CompleteOrchestrationIdempotencyRecord(ctx, sqlc.CompleteOrchestrationIdempotencyRecordParams{
		ResponsePayload: marshalJSON(result),
		TenantID:        caller.TenantID,
		CallerSubject:   caller.Subject,
		Method:          methodCreateHumanCheckpoint,
		TargetID:        pgTaskID.String(),
		IdempotencyKey:  normalizedIdempotencyKey,
	}); err != nil {
		return nil, fmt.Errorf("complete create checkpoint idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create checkpoint tx: %w", err)
	}
	return &result, nil
}

func (s *Service) CommitArtifact(ctx context.Context, caller ControlIdentity, req CommitArtifactRequest) (*CommitArtifactResult, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	normalizedIdempotencyKey := normalizeIdempotencyKey(req.IdempotencyKey)
	if strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.TaskID) == "" {
		return nil, fmt.Errorf("%w: run_id and task_id are required", ErrInvalidArgument)
	}
	if strings.TrimSpace(req.Kind) == "" || strings.TrimSpace(req.URI) == "" || strings.TrimSpace(req.Version) == "" || strings.TrimSpace(req.Digest) == "" {
		return nil, fmt.Errorf("%w: kind, uri, version, and digest are required", ErrInvalidArgument)
	}
	if normalizedIdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidArgument)
	}

	pgRunID, err := db.ParseUUID(req.RunID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run id", ErrInvalidArgument)
	}
	pgTaskID, err := db.ParseUUID(req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid task id", ErrInvalidArgument)
	}
	attemptID := pgtype.UUID{}
	if strings.TrimSpace(req.AttemptID) != "" {
		attemptID, err = db.ParseUUID(req.AttemptID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid attempt id", ErrInvalidArgument)
		}
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin commit artifact tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	lockedRun, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, pgRunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for artifact commit: %w", err)
	}
	if err := authorizeRun(caller, lockedRun); err != nil {
		return nil, ErrRunNotFound
	}
	if !runAcceptsExternalMutations(lockedRun.LifecycleStatus) {
		return nil, ErrRunImmutable
	}
	lockedTask, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, pgTaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock task for artifact commit: %w", err)
	}
	if lockedTask.RunID != lockedRun.ID {
		return nil, ErrTaskNotFound
	}
	if attemptID.Valid {
		attemptRow, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, attemptID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrAttemptNotFound
			}
			return nil, fmt.Errorf("lock attempt for artifact commit: %w", err)
		}
		if attemptRow.RunID != lockedRun.ID || attemptRow.TaskID != lockedTask.ID {
			return nil, ErrAttemptNotFound
		}
	}

	requestHash, err := commitArtifactRequestHash(req, normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}
	existing, existingFound, err := getIdempotencyRecord(ctx, qtx, caller, methodCommitArtifact, pgTaskID.String(), normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existingFound {
		if existing.RequestHash != requestHash {
			return nil, ErrIdempotencyConflict
		}
		if existing.State != "completed" {
			return nil, ErrIdempotencyIncomplete
		}
		result, err := decodeCommitArtifactResult(existing.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed artifact tx: %w", err)
		}
		return &result, nil
	}

	record, replay, err := ensureIdempotencyRecord(ctx, qtx, caller, methodCommitArtifact, pgTaskID.String(), normalizedIdempotencyKey, requestHash)
	if err != nil {
		return nil, err
	}
	if replay {
		result, err := decodeCommitArtifactResult(record.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed artifact tx: %w", err)
		}
		return &result, nil
	}

	artifactID, artifactUUID, err := newPGUUID()
	if err != nil {
		return nil, err
	}
	artifactRow, err := qtx.CreateOrchestrationArtifact(ctx, sqlc.CreateOrchestrationArtifactParams{
		ID:          artifactUUID,
		RunID:       pgRunID,
		TaskID:      pgTaskID,
		AttemptID:   attemptID,
		Kind:        strings.TrimSpace(req.Kind),
		Uri:         strings.TrimSpace(req.URI),
		Version:     strings.TrimSpace(req.Version),
		Digest:      strings.TrimSpace(req.Digest),
		ContentType: strings.TrimSpace(req.ContentType),
		Summary:     strings.TrimSpace(req.Summary),
		Metadata:    marshalObject(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create orchestration artifact: %w", err)
	}

	lastEvent, err := s.appendEvent(ctx, qtx, pgRunID, eventSpec{
		TaskID:           pgTaskID,
		CheckpointID:     pgtype.UUID{},
		AggregateType:    "artifact",
		AggregateID:      artifactUUID,
		AggregateVersion: 1,
		Type:             "run.event.artifact.committed",
		IdempotencyKey:   normalizedIdempotencyKey,
		Payload: map[string]any{
			"artifact_id":  artifactID,
			"run_id":       pgRunID.String(),
			"task_id":      pgTaskID.String(),
			"attempt_id":   attemptID.String(),
			"kind":         artifactRow.Kind,
			"uri":          artifactRow.Uri,
			"version":      artifactRow.Version,
			"digest":       artifactRow.Digest,
			"content_type": artifactRow.ContentType,
			"summary":      artifactRow.Summary,
			"metadata":     normalizeObject(req.Metadata),
		},
	})
	if err != nil {
		return nil, err
	}

	snapshotSeq, err := uint64FromInt64(lastEvent.Seq, "artifact commit event seq")
	if err != nil {
		return nil, err
	}
	result := CommitArtifactResult{
		Artifact:    toArtifact(artifactRow),
		SnapshotSeq: snapshotSeq,
	}
	if _, err := qtx.CompleteOrchestrationIdempotencyRecord(ctx, sqlc.CompleteOrchestrationIdempotencyRecordParams{
		ResponsePayload: marshalJSON(result),
		TenantID:        caller.TenantID,
		CallerSubject:   caller.Subject,
		Method:          methodCommitArtifact,
		TargetID:        pgTaskID.String(),
		IdempotencyKey:  normalizedIdempotencyKey,
	}); err != nil {
		return nil, fmt.Errorf("complete artifact idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit artifact tx: %w", err)
	}
	return &result, nil
}

type eventSpec struct {
	TaskID           pgtype.UUID
	AttemptID        pgtype.UUID
	CheckpointID     pgtype.UUID
	AggregateType    string
	AggregateID      pgtype.UUID
	AggregateVersion int64
	Type             string
	IdempotencyKey   string
	Payload          map[string]any
}

func (*Service) appendEvent(ctx context.Context, qtx *sqlc.Queries, runID pgtype.UUID, event eventSpec) (sqlc.OrchestrationEvent, error) {
	lastSeq, err := qtx.AllocateOrchestrationRunEventSeqs(ctx, sqlc.AllocateOrchestrationRunEventSeqsParams{
		Delta: 1,
		ID:    runID,
	})
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("allocate orchestration event seq: %w", err)
	}
	row, err := qtx.CreateOrchestrationEvent(ctx, sqlc.CreateOrchestrationEventParams{
		RunID:            runID,
		TaskID:           event.TaskID,
		AttemptID:        event.AttemptID,
		CheckpointID:     event.CheckpointID,
		Seq:              lastSeq,
		AggregateType:    event.AggregateType,
		AggregateID:      event.AggregateID,
		AggregateVersion: event.AggregateVersion,
		Type:             event.Type,
		CausationEventID: pgtype.UUID{},
		CorrelationID:    runID.String(),
		IdempotencyKey:   event.IdempotencyKey,
		Payload:          marshalObject(event.Payload),
	})
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("append orchestration event %q: %w", event.Type, err)
	}
	seq, err := uint64FromInt64(row.Seq, "event seq")
	if err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	if err := recordProjectionSnapshotAtSeq(ctx, qtx, runID, seq); err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	return row, nil
}

func recordProjectionSnapshotAtSeq(ctx context.Context, qtx *sqlc.Queries, runID pgtype.UUID, seq uint64) error {
	seqInt64, err := int64FromUint64(seq, "projection snapshot seq")
	if err != nil {
		return err
	}
	tasks, err := qtx.ListCurrentOrchestrationTasksByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("load tasks for projection snapshot: %w", err)
	}
	taskItems := make([]Task, 0, len(tasks))
	for _, item := range tasks {
		taskItems = append(taskItems, toTask(item))
	}
	checkpoints, err := qtx.ListCurrentOrchestrationCheckpointsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("load checkpoints for projection snapshot: %w", err)
	}
	checkpointItems := make([]HumanCheckpoint, 0, len(checkpoints))
	for _, item := range checkpoints {
		checkpointItems = append(checkpointItems, toHumanCheckpoint(item))
	}
	artifacts, err := qtx.ListCurrentOrchestrationArtifactsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("load artifacts for projection snapshot: %w", err)
	}
	artifactItems := make([]Artifact, 0, len(artifacts))
	for _, item := range artifacts {
		artifactItems = append(artifactItems, toArtifact(item))
	}
	runRow, err := qtx.GetOrchestrationRunByID(ctx, runID)
	if err != nil {
		return fmt.Errorf("load run for projection snapshot: %w", err)
	}
	runItem := toRun(runRow)
	if _, err := qtx.CreateOrchestrationProjectionSnapshot(ctx, sqlc.CreateOrchestrationProjectionSnapshotParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindRun,
		Seq:            seqInt64,
		Payload:        marshalJSON(runItem),
	}); err != nil {
		return fmt.Errorf("write run projection snapshot at seq %d: %w", seq, err)
	}
	if _, err := qtx.CreateOrchestrationProjectionSnapshot(ctx, sqlc.CreateOrchestrationProjectionSnapshotParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindTasks,
		Seq:            seqInt64,
		Payload:        marshalJSON(taskItems),
	}); err != nil {
		return fmt.Errorf("write task projection snapshot at seq %d: %w", seq, err)
	}
	if _, err := qtx.CreateOrchestrationProjectionSnapshot(ctx, sqlc.CreateOrchestrationProjectionSnapshotParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindCheckpoints,
		Seq:            seqInt64,
		Payload:        marshalJSON(checkpointItems),
	}); err != nil {
		return fmt.Errorf("write checkpoint projection snapshot at seq %d: %w", seq, err)
	}
	if _, err := qtx.CreateOrchestrationProjectionSnapshot(ctx, sqlc.CreateOrchestrationProjectionSnapshotParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindArtifacts,
		Seq:            seqInt64,
		Payload:        marshalJSON(artifactItems),
	}); err != nil {
		return fmt.Errorf("write artifact projection snapshot at seq %d: %w", seq, err)
	}
	return nil
}

func (s *Service) loadRunSnapshot(ctx context.Context, runID pgtype.UUID, asOfSeq uint64) (Run, uint64, error) {
	asOfSeqInt64, err := int64FromUint64(asOfSeq, "run snapshot seq")
	if err != nil {
		return Run{}, 0, err
	}
	row, err := s.queries.GetOrchestrationProjectionSnapshotAtOrBeforeSeq(ctx, sqlc.GetOrchestrationProjectionSnapshotAtOrBeforeSeqParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindRun,
		Seq:            asOfSeqInt64,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, 0, nil
		}
		return Run{}, 0, fmt.Errorf("load run projection snapshot: %w", err)
	}
	var item Run
	if err := unmarshalJSON(row.Payload, &item); err != nil {
		return Run{}, 0, fmt.Errorf("decode run projection snapshot: %w", err)
	}
	seq, err := uint64FromInt64(row.Seq, "run snapshot seq")
	if err != nil {
		return Run{}, 0, err
	}
	return item, seq, nil
}

func (s *Service) loadTaskSnapshot(ctx context.Context, runID pgtype.UUID, asOfSeq uint64) ([]Task, uint64, error) {
	asOfSeqInt64, err := int64FromUint64(asOfSeq, "task snapshot seq")
	if err != nil {
		return nil, 0, err
	}
	row, err := s.queries.GetOrchestrationProjectionSnapshotAtOrBeforeSeq(ctx, sqlc.GetOrchestrationProjectionSnapshotAtOrBeforeSeqParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindTasks,
		Seq:            asOfSeqInt64,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []Task{}, 0, nil
		}
		return nil, 0, fmt.Errorf("load task projection snapshot: %w", err)
	}
	var items []Task
	if err := unmarshalJSON(row.Payload, &items); err != nil {
		return nil, 0, fmt.Errorf("decode task projection snapshot: %w", err)
	}
	seq, err := uint64FromInt64(row.Seq, "task snapshot seq")
	if err != nil {
		return nil, 0, err
	}
	return items, seq, nil
}

func (s *Service) loadCheckpointSnapshot(ctx context.Context, runID pgtype.UUID, asOfSeq uint64) ([]HumanCheckpoint, uint64, error) {
	asOfSeqInt64, err := int64FromUint64(asOfSeq, "checkpoint snapshot seq")
	if err != nil {
		return nil, 0, err
	}
	row, err := s.queries.GetOrchestrationProjectionSnapshotAtOrBeforeSeq(ctx, sqlc.GetOrchestrationProjectionSnapshotAtOrBeforeSeqParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindCheckpoints,
		Seq:            asOfSeqInt64,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []HumanCheckpoint{}, 0, nil
		}
		return nil, 0, fmt.Errorf("load checkpoint projection snapshot: %w", err)
	}
	var items []HumanCheckpoint
	if err := unmarshalJSON(row.Payload, &items); err != nil {
		return nil, 0, fmt.Errorf("decode checkpoint projection snapshot: %w", err)
	}
	seq, err := uint64FromInt64(row.Seq, "checkpoint snapshot seq")
	if err != nil {
		return nil, 0, err
	}
	return items, seq, nil
}

func (s *Service) loadArtifactSnapshot(ctx context.Context, runID pgtype.UUID, asOfSeq uint64) ([]Artifact, uint64, error) {
	asOfSeqInt64, err := int64FromUint64(asOfSeq, "artifact snapshot seq")
	if err != nil {
		return nil, 0, err
	}
	row, err := s.queries.GetOrchestrationProjectionSnapshotAtOrBeforeSeq(ctx, sqlc.GetOrchestrationProjectionSnapshotAtOrBeforeSeqParams{
		RunID:          runID,
		ProjectionKind: ProjectionKindArtifacts,
		Seq:            asOfSeqInt64,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []Artifact{}, 0, nil
		}
		return nil, 0, fmt.Errorf("load artifact projection snapshot: %w", err)
	}
	var items []Artifact
	if err := unmarshalJSON(row.Payload, &items); err != nil {
		return nil, 0, fmt.Errorf("decode artifact projection snapshot: %w", err)
	}
	seq, err := uint64FromInt64(row.Seq, "artifact snapshot seq")
	if err != nil {
		return nil, 0, err
	}
	return items, seq, nil
}

func (s *Service) getAuthorizedRun(ctx context.Context, caller ControlIdentity, runID string) (sqlc.OrchestrationRun, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return sqlc.OrchestrationRun{}, err
	}
	pgRunID, err := db.ParseUUID(runID)
	if err != nil {
		return sqlc.OrchestrationRun{}, fmt.Errorf("%w: invalid run id", ErrInvalidArgument)
	}
	row, err := s.queries.GetOrchestrationRunByID(ctx, pgRunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationRun{}, ErrRunNotFound
		}
		return sqlc.OrchestrationRun{}, fmt.Errorf("get orchestration run: %w", err)
	}
	if err := authorizeRun(caller, row); err != nil {
		return sqlc.OrchestrationRun{}, ErrRunNotFound
	}
	return row, nil
}

func normalizeControlIdentity(caller ControlIdentity) (ControlIdentity, error) {
	caller.Subject = strings.TrimSpace(caller.Subject)
	caller.TenantID = strings.TrimSpace(caller.TenantID)
	if caller.Subject == "" || caller.TenantID == "" {
		return ControlIdentity{}, ErrInvalidControlIdentity
	}
	return caller, nil
}

func authorizeRun(caller ControlIdentity, run sqlc.OrchestrationRun) error {
	if strings.TrimSpace(run.TenantID) != strings.TrimSpace(caller.TenantID) {
		return ErrAccessDenied
	}
	policy := decodeJSONObject(run.ControlPolicy)
	mode, _ := policy["mode"].(string)
	ownerSubject, _ := policy["owner_subject"].(string)
	ownerSubject = strings.TrimSpace(ownerSubject)
	if ownerSubject == "" {
		ownerSubject = run.OwnerSubject
	}
	runOwner := strings.TrimSpace(run.OwnerSubject)
	if mode == ControlPolicyModeOwnerOnly && ownerSubject == strings.TrimSpace(caller.Subject) {
		return nil
	}
	if runOwner != strings.TrimSpace(caller.Subject) {
		return ErrAccessDenied
	}
	return nil
}

func runAcceptsExternalMutations(status string) bool {
	switch status {
	case LifecycleStatusCreated, LifecycleStatusRunning, LifecycleStatusWaitingHuman:
		return true
	case LifecycleStatusCancelling, LifecycleStatusCompleted, LifecycleStatusFailed, LifecycleStatusCancelled:
		return false
	default:
		return false
	}
}

func runAcceptsPlanningIntent(kind, status string, basePlannerEpoch, currentPlannerEpoch int64) bool {
	if !runAcceptsExternalMutations(status) {
		return false
	}
	if kind == PlanningIntentKindCheckpointResume {
		return true
	}
	return basePlannerEpoch == currentPlannerEpoch
}

func normalizeOptionalUUID(raw string, field string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := db.ParseUUID(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: invalid %s", ErrInvalidArgument, field)
	}
	return parsed.String(), nil
}

func taskAcceptsHumanCheckpoint(status string) bool {
	switch status {
	case TaskStatusCreated, TaskStatusReady, TaskStatusDispatching, TaskStatusRunning, TaskStatusVerifying, TaskStatusWaitingHuman, TaskStatusBlocked:
		return true
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return false
	default:
		return false
	}
}

func taskSupportsHumanCheckpoint(status string) bool {
	return status == TaskStatusReady
}

func taskSuperseded(task sqlc.OrchestrationTask) bool {
	return task.SupersededByPlannerEpoch.Valid
}

func taskPauseableByRunBarrier(task sqlc.OrchestrationTask) bool {
	if taskSuperseded(task) {
		return false
	}
	if task.WaitingCheckpointID.Valid {
		return false
	}
	switch task.Status {
	case TaskStatusReady, TaskStatusDispatching, TaskStatusRunning, TaskStatusVerifying:
		return true
	default:
		return false
	}
}

func taskBlocksRunBarrier(task sqlc.OrchestrationTask) bool {
	if taskSuperseded(task) {
		return false
	}
	switch task.Status {
	case TaskStatusWaitingHuman:
		return true
	case TaskStatusDispatching, TaskStatusRunning, TaskStatusVerifying:
		return false
	case TaskStatusCreated, TaskStatusReady, TaskStatusBlocked, TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return false
	default:
		return true
	}
}

func taskWaitingOnRunBarrier(task sqlc.OrchestrationTask, checkpointID pgtype.UUID) bool {
	return task.Status == TaskStatusWaitingHuman &&
		task.WaitingScope == "run" &&
		task.WaitingCheckpointID.Valid &&
		task.WaitingCheckpointID == checkpointID
}

func (s *Service) pauseRunBarrierSiblingWork(
	ctx context.Context,
	qtx *sqlc.Queries,
	runID pgtype.UUID,
	checkpointID pgtype.UUID,
	taskRow sqlc.OrchestrationTask,
	runAttempts []sqlc.OrchestrationTaskAttempt,
	runVerifications []sqlc.OrchestrationTaskVerification,
) error {
	if taskRow.Status == TaskStatusReady {
		return nil
	}
	if taskRow.Status == TaskStatusVerifying {
		return s.pauseRunBarrierSiblingVerification(ctx, qtx, runID, checkpointID, taskRow, runVerifications)
	}
	pausedAttempt := false
	for _, attempt := range runAttempts {
		if attempt.TaskID != taskRow.ID || isTerminalAttemptStatus(attempt.Status) {
			continue
		}
		pausedAttempt = true
		lockedAttempt, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, attempt.ID)
		if err != nil {
			return fmt.Errorf("lock sibling attempt for run barrier: %w", err)
		}
		if isTerminalAttemptStatus(lockedAttempt.Status) {
			continue
		}
		var finalAttempt sqlc.OrchestrationTaskAttempt
		switch lockedAttempt.Status {
		case TaskAttemptStatusCreated:
			finalAttempt, err = qtx.RetireCreatedOrchestrationTaskAttemptFailed(ctx, sqlc.RetireCreatedOrchestrationTaskAttemptFailedParams{
				ID:             lockedAttempt.ID,
				FailureClass:   "run_barrier_paused",
				TerminalReason: "attempt paused by run barrier before execution start",
			})
			if err == nil {
				_, err = s.appendEvent(ctx, qtx, runID, eventSpec{
					TaskID:           finalAttempt.TaskID,
					AttemptID:        finalAttempt.ID,
					CheckpointID:     checkpointID,
					AggregateType:    "attempt",
					AggregateID:      finalAttempt.ID,
					AggregateVersion: finalAttempt.ClaimEpoch,
					Type:             "run.event.attempt.failed",
					Payload: map[string]any{
						"attempt_id":      finalAttempt.ID.String(),
						"task_id":         finalAttempt.TaskID.String(),
						"previous_status": lockedAttempt.Status,
						"new_status":      finalAttempt.Status,
						"failure_class":   finalAttempt.FailureClass,
						"terminal_reason": finalAttempt.TerminalReason,
					},
				})
			}
		case TaskAttemptStatusRunning:
			finalAttempt, err = qtx.PreemptRunningOrchestrationTaskAttemptFailed(ctx, sqlc.PreemptRunningOrchestrationTaskAttemptFailedParams{
				ID:             lockedAttempt.ID,
				ClaimEpoch:     lockedAttempt.ClaimEpoch,
				FailureClass:   "run_barrier_preempted",
				TerminalReason: "attempt preempted by run barrier during execution",
			})
			if err == nil {
				_, err = s.appendEvent(ctx, qtx, runID, eventSpec{
					TaskID:           finalAttempt.TaskID,
					AttemptID:        finalAttempt.ID,
					CheckpointID:     checkpointID,
					AggregateType:    "attempt",
					AggregateID:      finalAttempt.ID,
					AggregateVersion: finalAttempt.ClaimEpoch,
					Type:             "run.event.attempt.failed",
					Payload: map[string]any{
						"attempt_id":      finalAttempt.ID.String(),
						"task_id":         finalAttempt.TaskID.String(),
						"previous_status": lockedAttempt.Status,
						"new_status":      finalAttempt.Status,
						"failure_class":   finalAttempt.FailureClass,
						"terminal_reason": finalAttempt.TerminalReason,
					},
				})
			}
		case TaskAttemptStatusClaimed, TaskAttemptStatusBinding:
			_, err = s.retireAttempt(ctx, qtx, lockedAttempt, attemptRetirementSpec{
				Status:         TaskAttemptStatusFailed,
				FailureClass:   "run_barrier_paused",
				TerminalReason: "attempt paused by run barrier before execution start",
			})
		}
		if err != nil {
			return fmt.Errorf("pause sibling attempt for run barrier: %w", err)
		}
	}
	if !pausedAttempt {
		return ErrRunBarrierUnsupported
	}
	return nil
}

func (s *Service) pauseRunBarrierSiblingVerification(
	ctx context.Context,
	qtx *sqlc.Queries,
	runID pgtype.UUID,
	checkpointID pgtype.UUID,
	taskRow sqlc.OrchestrationTask,
	runVerifications []sqlc.OrchestrationTaskVerification,
) error {
	for _, verification := range runVerifications {
		if verification.TaskID != taskRow.ID || isTerminalVerificationStatus(verification.Status) {
			continue
		}
		if verification.Status == TaskVerificationStatusCreated {
			return nil
		}
		lockedVerification, err := qtx.GetOrchestrationTaskVerificationByIDForUpdate(ctx, verification.ID)
		if err != nil {
			return fmt.Errorf("lock sibling verification for run barrier: %w", err)
		}
		if isTerminalVerificationStatus(lockedVerification.Status) {
			return nil
		}
		requeuedVerification, err := qtx.RequeueOrchestrationTaskVerification(ctx, sqlc.RequeueOrchestrationTaskVerificationParams{
			ID:         lockedVerification.ID,
			ClaimEpoch: lockedVerification.ClaimEpoch,
		})
		if err != nil {
			return fmt.Errorf("requeue sibling verification for run barrier: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, runID, eventSpec{
			TaskID:           requeuedVerification.TaskID,
			CheckpointID:     checkpointID,
			AggregateType:    "verification",
			AggregateID:      requeuedVerification.ID,
			AggregateVersion: requeuedVerification.ClaimEpoch,
			Type:             "run.event.verification.requeued",
			Payload: map[string]any{
				"verification_id": requeuedVerification.ID.String(),
				"task_id":         requeuedVerification.TaskID.String(),
				"previous_status": lockedVerification.Status,
				"new_status":      requeuedVerification.Status,
				"result_id":       requeuedVerification.ResultID.String(),
				"terminal_reason": "verification preempted by run barrier",
			},
		}); err != nil {
			return err
		}
		return nil
	}
	return ErrRunBarrierUnsupported
}

func findOpenRunBlockingCheckpoint(ctx context.Context, qtx *sqlc.Queries, runID pgtype.UUID) (sqlc.OrchestrationHumanCheckpoint, bool, error) {
	checkpoints, err := qtx.ListCurrentOrchestrationCheckpointsByRun(ctx, runID)
	if err != nil {
		return sqlc.OrchestrationHumanCheckpoint{}, false, err
	}
	for _, checkpoint := range checkpoints {
		if checkpoint.BlocksRun && checkpoint.Status == CheckpointStatusOpen {
			return checkpoint, true, nil
		}
	}
	return sqlc.OrchestrationHumanCheckpoint{}, false, nil
}

func normalizeIdempotencyKey(value string) string {
	return strings.TrimSpace(value)
}

func createHumanCheckpointRequestHash(
	req CreateHumanCheckpointRequest,
	options []CheckpointOption,
	defaultAction *CheckpointDefaultAction,
	resumePolicy *CheckpointResumePolicy,
	idempotencyKey string,
) (string, error) {
	runID, err := normalizeOptionalUUID(req.RunID, "run_id")
	if err != nil {
		return "", err
	}
	taskID, err := normalizeOptionalUUID(req.TaskID, "task_id")
	if err != nil {
		return "", err
	}
	var timeoutAt *time.Time
	if !req.TimeoutAt.IsZero() {
		value := req.TimeoutAt.UTC()
		timeoutAt = &value
	}
	normalized := struct {
		RunID          string                   `json:"run_id"`
		TaskID         string                   `json:"task_id"`
		BlocksRun      bool                     `json:"blocks_run"`
		Question       string                   `json:"question"`
		Options        []CheckpointOption       `json:"options"`
		DefaultAction  *CheckpointDefaultAction `json:"default_action"`
		ResumePolicy   *CheckpointResumePolicy  `json:"resume_policy"`
		TimeoutAt      *time.Time               `json:"timeout_at"`
		Metadata       map[string]any           `json:"metadata"`
		IdempotencyKey string                   `json:"idempotency_key"`
	}{
		RunID:          runID,
		TaskID:         taskID,
		BlocksRun:      req.BlocksRun,
		Question:       strings.TrimSpace(req.Question),
		Options:        options,
		DefaultAction:  defaultAction,
		ResumePolicy:   resumePolicy,
		TimeoutAt:      timeoutAt,
		Metadata:       normalizeObject(req.Metadata),
		IdempotencyKey: idempotencyKey,
	}
	return hashJSON(normalized)
}

func commitArtifactRequestHash(req CommitArtifactRequest, idempotencyKey string) (string, error) {
	runID, err := normalizeOptionalUUID(req.RunID, "run_id")
	if err != nil {
		return "", err
	}
	taskID, err := normalizeOptionalUUID(req.TaskID, "task_id")
	if err != nil {
		return "", err
	}
	attemptID, err := normalizeOptionalUUID(req.AttemptID, "attempt_id")
	if err != nil {
		return "", err
	}
	normalized := struct {
		RunID          string         `json:"run_id"`
		TaskID         string         `json:"task_id"`
		AttemptID      string         `json:"attempt_id"`
		Kind           string         `json:"kind"`
		URI            string         `json:"uri"`
		Version        string         `json:"version"`
		Digest         string         `json:"digest"`
		ContentType    string         `json:"content_type"`
		Summary        string         `json:"summary"`
		Metadata       map[string]any `json:"metadata"`
		IdempotencyKey string         `json:"idempotency_key"`
	}{
		RunID:          runID,
		TaskID:         taskID,
		AttemptID:      attemptID,
		Kind:           strings.TrimSpace(req.Kind),
		URI:            strings.TrimSpace(req.URI),
		Version:        strings.TrimSpace(req.Version),
		Digest:         strings.TrimSpace(req.Digest),
		ContentType:    strings.TrimSpace(req.ContentType),
		Summary:        strings.TrimSpace(req.Summary),
		Metadata:       normalizeObject(req.Metadata),
		IdempotencyKey: idempotencyKey,
	}
	return hashJSON(normalized)
}

func startRunRequestHash(req StartRunRequest) (string, error) {
	normalized := struct {
		Goal                   string         `json:"goal"`
		Input                  map[string]any `json:"input"`
		OutputSchema           map[string]any `json:"output_schema"`
		IdempotencyKey         string         `json:"idempotency_key"`
		RequestedControlPolicy map[string]any `json:"requested_control_policy"`
		SourceMetadata         map[string]any `json:"source_metadata"`
		Policies               map[string]any `json:"policies"`
	}{
		Goal:                   strings.TrimSpace(req.Goal),
		Input:                  normalizeObject(req.Input),
		OutputSchema:           normalizeObject(req.OutputSchema),
		IdempotencyKey:         normalizeIdempotencyKey(req.IdempotencyKey),
		RequestedControlPolicy: normalizeObject(req.RequestedControlPolicy),
		SourceMetadata:         normalizeObject(req.SourceMetadata),
		Policies:               normalizeObject(req.Policies),
	}
	return hashJSON(normalized)
}

func buildPhase1ControlPolicy(caller ControlIdentity) map[string]any {
	return map[string]any{
		"mode":          ControlPolicyModeOwnerOnly,
		"owner_subject": caller.Subject,
	}
}

func normalizeCheckpointResolution(checkpoint sqlc.OrchestrationHumanCheckpoint, input CheckpointResolution) (CheckpointResolution, string, error) {
	options, err := decodeCheckpointOptions(checkpoint.Options)
	if err != nil {
		return CheckpointResolution{}, "", fmt.Errorf("%w: invalid checkpoint options", ErrInvalidCheckpointResolution)
	}
	defaultAction, err := decodeCheckpointDefaultAction(checkpoint.DefaultAction)
	if err != nil {
		return CheckpointResolution{}, "", fmt.Errorf("%w: invalid checkpoint default_action", ErrInvalidCheckpointResolution)
	}

	resolution := CheckpointResolution{
		Mode:           strings.TrimSpace(input.Mode),
		OptionID:       strings.TrimSpace(input.OptionID),
		FreeformInput:  input.FreeformInput,
		Metadata:       normalizeObject(input.Metadata),
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
	}
	if resolution.Mode == "" {
		return CheckpointResolution{}, "", fmt.Errorf("%w: mode is required", ErrInvalidCheckpointResolution)
	}

	if resolution.Mode == CheckpointResolutionModeUseDefault {
		if defaultAction == nil {
			return CheckpointResolution{}, "", fmt.Errorf("%w: checkpoint has no default_action", ErrInvalidCheckpointResolution)
		}
		resolution.Mode = strings.TrimSpace(defaultAction.Mode)
		resolution.OptionID = strings.TrimSpace(defaultAction.OptionID)
		resolution.FreeformInput = defaultAction.FreeformInput
	}

	option, ok := findCheckpointOption(options, resolution.OptionID)
	switch resolution.Mode {
	case CheckpointResolutionModeSelectOption:
		if !ok || option.Kind != CheckpointOptionKindChoice {
			return CheckpointResolution{}, "", fmt.Errorf("%w: option_id must target a choice option", ErrInvalidCheckpointResolution)
		}
		if strings.TrimSpace(resolution.FreeformInput) != "" {
			return CheckpointResolution{}, "", fmt.Errorf("%w: freeform_input must be empty for select_option", ErrInvalidCheckpointResolution)
		}
		resolution.FreeformInput = ""
	case CheckpointResolutionModeFreeform:
		if !ok || option.Kind != CheckpointOptionKindFreeform {
			return CheckpointResolution{}, "", fmt.Errorf("%w: option_id must target a freeform option", ErrInvalidCheckpointResolution)
		}
		if strings.TrimSpace(resolution.FreeformInput) == "" {
			return CheckpointResolution{}, "", fmt.Errorf("%w: freeform_input is required for freeform", ErrInvalidCheckpointResolution)
		}
	default:
		return CheckpointResolution{}, "", fmt.Errorf("%w: unsupported resolution mode %q", ErrInvalidCheckpointResolution, resolution.Mode)
	}

	hash, err := hashJSON(struct {
		Mode           string         `json:"mode"`
		OptionID       string         `json:"option_id"`
		FreeformInput  string         `json:"freeform_input"`
		Metadata       map[string]any `json:"metadata"`
		IdempotencyKey string         `json:"idempotency_key"`
	}{
		Mode:           resolution.Mode,
		OptionID:       resolution.OptionID,
		FreeformInput:  resolution.FreeformInput,
		Metadata:       resolution.Metadata,
		IdempotencyKey: resolution.IdempotencyKey,
	})
	if err != nil {
		return CheckpointResolution{}, "", err
	}
	return resolution, hash, nil
}

func normalizeCheckpointResumePolicy(input *CheckpointResumePolicy) (*CheckpointResumePolicy, error) {
	if input == nil {
		return nil, nil
	}
	normalized := &CheckpointResumePolicy{
		ResumeMode: strings.TrimSpace(input.ResumeMode),
	}
	switch normalized.ResumeMode {
	case CheckpointResumeModeNewAttempt:
		return normalized, nil
	case CheckpointResumeModeResumeHeldEnv:
		return nil, fmt.Errorf("%w: resume_policy.resume_mode %q is not supported in current runtime", ErrInvalidArgument, normalized.ResumeMode)
	case "":
		return nil, fmt.Errorf("%w: resume_policy.resume_mode is required", ErrInvalidArgument)
	default:
		return nil, fmt.Errorf("%w: unsupported resume_policy.resume_mode %q", ErrInvalidArgument, normalized.ResumeMode)
	}
}

func validateCheckpointDefinition(options []CheckpointOption, defaultAction *CheckpointDefaultAction, timeoutAt time.Time) ([]CheckpointOption, *CheckpointDefaultAction, error) {
	if len(options) == 0 {
		return nil, nil, fmt.Errorf("%w: checkpoint options are required", ErrInvalidArgument)
	}
	normalizedOptions := make([]CheckpointOption, 0, len(options))
	seenOptionIDs := make(map[string]struct{}, len(options))
	for _, option := range options {
		normalized := CheckpointOption{
			ID:          strings.TrimSpace(option.ID),
			Kind:        strings.TrimSpace(option.Kind),
			Label:       option.Label,
			Description: option.Description,
		}
		switch normalized.Kind {
		case CheckpointOptionKindChoice, CheckpointOptionKindFreeform:
		default:
			return nil, nil, fmt.Errorf("%w: unsupported checkpoint option kind %q", ErrInvalidArgument, option.Kind)
		}
		if normalized.ID == "" {
			return nil, nil, fmt.Errorf("%w: checkpoint option id is required", ErrInvalidArgument)
		}
		if _, exists := seenOptionIDs[normalized.ID]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate checkpoint option id %q", ErrInvalidArgument, normalized.ID)
		}
		seenOptionIDs[normalized.ID] = struct{}{}
		normalizedOptions = append(normalizedOptions, normalized)
	}
	if defaultAction == nil {
		if !timeoutAt.IsZero() {
			return nil, nil, fmt.Errorf("%w: timeout requires default_action", ErrInvalidArgument)
		}
		return normalizedOptions, nil, nil
	}
	normalized := &CheckpointDefaultAction{
		Mode:          strings.TrimSpace(defaultAction.Mode),
		OptionID:      strings.TrimSpace(defaultAction.OptionID),
		FreeformInput: defaultAction.FreeformInput,
	}
	option, ok := findCheckpointOption(normalizedOptions, normalized.OptionID)
	switch normalized.Mode {
	case CheckpointResolutionModeSelectOption:
		if !ok || option.Kind != CheckpointOptionKindChoice {
			return nil, nil, fmt.Errorf("%w: default select_option must target a choice option", ErrInvalidArgument)
		}
		if strings.TrimSpace(normalized.FreeformInput) != "" {
			return nil, nil, fmt.Errorf("%w: default select_option cannot include freeform_input", ErrInvalidArgument)
		}
	case CheckpointResolutionModeFreeform:
		if !ok || option.Kind != CheckpointOptionKindFreeform {
			return nil, nil, fmt.Errorf("%w: default freeform must target a freeform option", ErrInvalidArgument)
		}
		if strings.TrimSpace(normalized.FreeformInput) == "" {
			return nil, nil, fmt.Errorf("%w: default freeform requires freeform_input", ErrInvalidArgument)
		}
	default:
		return nil, nil, fmt.Errorf("%w: unsupported default_action mode %q", ErrInvalidArgument, normalized.Mode)
	}
	return normalizedOptions, normalized, nil
}

func findCheckpointOption(options []CheckpointOption, id string) (CheckpointOption, bool) {
	needle := strings.TrimSpace(id)
	for _, option := range options {
		if strings.TrimSpace(option.ID) == needle {
			return option, true
		}
	}
	return CheckpointOption{}, false
}

func ensureIdempotencyRecord(ctx context.Context, qtx *sqlc.Queries, caller ControlIdentity, method, targetID, idempotencyKey, requestHash string) (sqlc.OrchestrationIdempotencyRecord, bool, error) {
	record, err := qtx.TryCreateOrchestrationIdempotencyRecord(ctx, sqlc.TryCreateOrchestrationIdempotencyRecordParams{
		TenantID:       caller.TenantID,
		CallerSubject:  caller.Subject,
		Method:         method,
		TargetID:       targetID,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
	})
	if err == nil {
		return record, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sqlc.OrchestrationIdempotencyRecord{}, false, fmt.Errorf("create idempotency record: %w", err)
	}
	record, err = qtx.GetOrchestrationIdempotencyRecordForUpdate(ctx, sqlc.GetOrchestrationIdempotencyRecordForUpdateParams{
		TenantID:       caller.TenantID,
		CallerSubject:  caller.Subject,
		Method:         method,
		TargetID:       targetID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return sqlc.OrchestrationIdempotencyRecord{}, false, fmt.Errorf("load idempotency record: %w", err)
	}
	if record.RequestHash != requestHash {
		return sqlc.OrchestrationIdempotencyRecord{}, false, ErrIdempotencyConflict
	}
	if record.State != "completed" {
		return sqlc.OrchestrationIdempotencyRecord{}, false, ErrIdempotencyIncomplete
	}
	return record, true, nil
}

func getIdempotencyRecord(ctx context.Context, qtx *sqlc.Queries, caller ControlIdentity, method, targetID, idempotencyKey string) (sqlc.OrchestrationIdempotencyRecord, bool, error) {
	record, err := qtx.GetOrchestrationIdempotencyRecordForUpdate(ctx, sqlc.GetOrchestrationIdempotencyRecordForUpdateParams{
		TenantID:       caller.TenantID,
		CallerSubject:  caller.Subject,
		Method:         method,
		TargetID:       targetID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationIdempotencyRecord{}, false, nil
		}
		return sqlc.OrchestrationIdempotencyRecord{}, false, fmt.Errorf("get idempotency record: %w", err)
	}
	return record, true, nil
}

func decodeRunHandle(raw []byte) (RunHandle, error) {
	var handle RunHandle
	if err := unmarshalJSON(raw, &handle); err != nil {
		return RunHandle{}, fmt.Errorf("decode idempotent run handle: %w", err)
	}
	return handle, nil
}

func decodeCreateHumanCheckpointResult(raw []byte) (CreateHumanCheckpointResult, error) {
	var result CreateHumanCheckpointResult
	if err := unmarshalJSON(raw, &result); err != nil {
		return CreateHumanCheckpointResult{}, fmt.Errorf("decode idempotent create checkpoint result: %w", err)
	}
	return result, nil
}

func decodeResolveCheckpointResult(raw []byte) (ResolveCheckpointResult, error) {
	var result ResolveCheckpointResult
	if err := unmarshalJSON(raw, &result); err != nil {
		return ResolveCheckpointResult{}, fmt.Errorf("decode idempotent resolve checkpoint result: %w", err)
	}
	return result, nil
}

func decodeCommitArtifactResult(raw []byte) (CommitArtifactResult, error) {
	var result CommitArtifactResult
	if err := unmarshalJSON(raw, &result); err != nil {
		return CommitArtifactResult{}, fmt.Errorf("decode idempotent artifact result: %w", err)
	}
	return result, nil
}

type paginationCursor struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	SnapshotSeq uint64    `json:"snapshot_seq,omitempty"`
	FilterHash  string    `json:"filter_hash,omitempty"`
}

func encodeCursor(id string, createdAt time.Time, snapshotSeq uint64, filterHash string) string {
	return base64.RawURLEncoding.EncodeToString(marshalJSON(paginationCursor{
		ID:          id,
		CreatedAt:   createdAt.UTC(),
		SnapshotSeq: snapshotSeq,
		FilterHash:  filterHash,
	}))
}

func decodeCursor(raw string) (paginationCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return paginationCursor{}, ErrInvalidCursor
	}
	var cursor paginationCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return paginationCursor{}, ErrInvalidCursor
	}
	if strings.TrimSpace(cursor.ID) == "" || cursor.CreatedAt.IsZero() {
		return paginationCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

func paginateTasks(items []Task, after string, limit int, snapshotSeq uint64, filterHash string) ([]Task, string, error) {
	start, err := taskPageStart(items, after, filterHash)
	if err != nil {
		return nil, "", err
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	if end == len(items) {
		return page, "", nil
	}
	last := page[len(page)-1]
	return page, encodeCursor(last.ID, last.CreatedAt, snapshotSeq, filterHash), nil
}

func paginateCheckpoints(items []HumanCheckpoint, after string, limit int, snapshotSeq uint64, filterHash string) ([]HumanCheckpoint, string, error) {
	start, err := checkpointPageStart(items, after, filterHash)
	if err != nil {
		return nil, "", err
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	if end == len(items) {
		return page, "", nil
	}
	last := page[len(page)-1]
	return page, encodeCursor(last.ID, last.CreatedAt, snapshotSeq, filterHash), nil
}

func paginateArtifacts(items []Artifact, after string, limit int, snapshotSeq uint64, filterHash string) ([]Artifact, string, error) {
	start, err := artifactPageStart(items, after, filterHash)
	if err != nil {
		return nil, "", err
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	if end == len(items) {
		return page, "", nil
	}
	last := page[len(page)-1]
	return page, encodeCursor(last.ID, last.CreatedAt, snapshotSeq, filterHash), nil
}

func taskPageStart(items []Task, after string, filterHash string) (int, error) {
	if strings.TrimSpace(after) == "" {
		return 0, nil
	}
	cursor, err := decodeCursor(after)
	if err != nil {
		return 0, err
	}
	if cursor.FilterHash != filterHash {
		return 0, ErrInvalidCursor
	}
	for idx, item := range items {
		if item.ID == cursor.ID && item.CreatedAt.Equal(cursor.CreatedAt) {
			return idx + 1, nil
		}
	}
	return 0, ErrInvalidCursor
}

func checkpointPageStart(items []HumanCheckpoint, after string, filterHash string) (int, error) {
	if strings.TrimSpace(after) == "" {
		return 0, nil
	}
	cursor, err := decodeCursor(after)
	if err != nil {
		return 0, err
	}
	if cursor.FilterHash != filterHash {
		return 0, ErrInvalidCursor
	}
	for idx, item := range items {
		if item.ID == cursor.ID && item.CreatedAt.Equal(cursor.CreatedAt) {
			return idx + 1, nil
		}
	}
	return 0, ErrInvalidCursor
}

func artifactPageStart(items []Artifact, after string, filterHash string) (int, error) {
	if strings.TrimSpace(after) == "" {
		return 0, nil
	}
	cursor, err := decodeCursor(after)
	if err != nil {
		return 0, err
	}
	if cursor.FilterHash != filterHash {
		return 0, ErrInvalidCursor
	}
	for idx, item := range items {
		if item.ID == cursor.ID && item.CreatedAt.Equal(cursor.CreatedAt) {
			return idx + 1, nil
		}
	}
	return 0, ErrInvalidCursor
}

func filterTasks(items []Task, statuses []string) []Task {
	if len(statuses) == 0 {
		return items
	}
	allowed := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		trimmed := strings.TrimSpace(status)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return items
	}
	filtered := make([]Task, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Status]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterCheckpoints(items []HumanCheckpoint, statuses []string) []HumanCheckpoint {
	if len(statuses) == 0 {
		return items
	}
	allowed := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		trimmed := strings.TrimSpace(status)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return items
	}
	filtered := make([]HumanCheckpoint, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Status]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterArtifacts(items []Artifact, taskID string, kinds []string) []Artifact {
	trimmedTaskID := strings.TrimSpace(taskID)
	allowedKinds := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		trimmed := strings.TrimSpace(kind)
		if trimmed == "" {
			continue
		}
		allowedKinds[trimmed] = struct{}{}
	}
	filtered := make([]Artifact, 0, len(items))
	for _, item := range items {
		if trimmedTaskID != "" && item.TaskID != trimmedTaskID {
			continue
		}
		if len(allowedKinds) > 0 {
			if _, ok := allowedKinds[item.Kind]; !ok {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func filterHash(values ...[]string) string {
	parts := make([]string, 0, len(values))
	for _, group := range values {
		seen := make(map[string]struct{}, len(group))
		normalized := make([]string, 0, len(group))
		for _, value := range group {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			normalized = append(normalized, trimmed)
		}
		sort.Strings(normalized)
		parts = append(parts, strings.Join(normalized, "\x1f"))
	}
	return strings.Join(parts, "\x1e")
}

func normalizedListLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}

func normalizedEventLimit(limit int) int {
	if limit <= 0 {
		return defaultEventLimit
	}
	if limit > maxEventLimit {
		return maxEventLimit
	}
	return limit
}

func resolvePageAsOfSeq(current, requested uint64, after string) (uint64, error) {
	if strings.TrimSpace(after) != "" {
		cursor, err := decodeCursor(after)
		if err != nil {
			return 0, err
		}
		if cursor.SnapshotSeq != 0 {
			if requested != 0 && requested != cursor.SnapshotSeq {
				return 0, ErrInvalidCursor
			}
			requested = cursor.SnapshotSeq
		}
		if requested == 0 {
			return 0, ErrInvalidCursor
		}
	}
	if requested == 0 {
		return current, nil
	}
	if requested > current {
		return 0, fmt.Errorf("%w: as_of_seq exceeds current snapshot", ErrInvalidArgument)
	}
	return requested, nil
}

func resolveEventUntilSeq(current, after, requested uint64) (uint64, error) {
	if requested == 0 {
		if after > 0 {
			return 0, fmt.Errorf("%w: until_seq is required when after_seq is set", ErrInvalidArgument)
		}
		requested = current
	}
	if requested > current {
		return 0, fmt.Errorf("%w: until_seq exceeds current snapshot", ErrInvalidArgument)
	}
	if after > requested {
		return 0, fmt.Errorf("%w: after_seq exceeds until_seq", ErrInvalidArgument)
	}
	return requested, nil
}

func normalizeObject(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = normalizeJSONValue(value)
	}
	return out
}

func normalizeJSONValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return normalizeObject(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, normalizeJSONValue(item))
		}
		return out
	default:
		return normalizeTypedJSONValue(v)
	}
}

func normalizeTypedJSONValue(value any) any {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Map:
		if rv.IsNil() {
			return map[string]any{}
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[fmt.Sprint(iter.Key().Interface())] = normalizeJSONValue(iter.Value().Interface())
		}
		return out
	case reflect.Slice:
		if rv.IsNil() {
			return []any{}
		}
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return value
		}
		fallthrough
	case reflect.Array:
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out = append(out, normalizeJSONValue(rv.Index(i).Interface()))
		}
		return out
	default:
		return value
	}
}

func hashJSON(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal canonical json: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func marshalObject(value map[string]any) []byte {
	return marshalJSON(normalizeObject(value))
}

func marshalJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return payload
}

func unmarshalJSON(data []byte, target any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

func timeToPg(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func timeForJSON(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func newPGUUID() (string, pgtype.UUID, error) {
	raw := uuid.NewString()
	pgID, err := db.ParseUUID(raw)
	if err != nil {
		return "", pgtype.UUID{}, err
	}
	return raw, pgID, nil
}

func decodeCheckpointOptions(raw []byte) ([]CheckpointOption, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var options []CheckpointOption
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, err
	}
	return options, nil
}

func decodeCheckpointDefaultAction(raw []byte) (*CheckpointDefaultAction, error) {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return nil, nil
	}
	var action CheckpointDefaultAction
	if err := json.Unmarshal(raw, &action); err != nil {
		return nil, err
	}
	if strings.TrimSpace(action.Mode) == "" && strings.TrimSpace(action.OptionID) == "" && strings.TrimSpace(action.FreeformInput) == "" {
		return nil, nil
	}
	return &action, nil
}

func decodeCheckpointResumePolicy(raw []byte) (*CheckpointResumePolicy, error) {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return nil, nil
	}
	var policy CheckpointResumePolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return nil, err
	}
	return normalizeCheckpointResumePolicy(&policy)
}

func decodeJSONObject(raw []byte) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{}
	}
	return normalizeObject(value)
}

func uint64FromInt64(value int64, field string) (uint64, error) {
	parsed, err := strconv.ParseUint(strconv.FormatInt(value, 10), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s out of range (%w)", ErrInvalidArgument, field, err)
	}
	return parsed, nil
}

func int64FromUint64(value uint64, field string) (int64, error) {
	parsed, err := strconv.ParseInt(strconv.FormatUint(value, 10), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s out of range (%w)", ErrInvalidArgument, field, err)
	}
	return parsed, nil
}

func int32FromInt(value int, field string) (int32, error) {
	parsed, err := strconv.ParseInt(strconv.FormatInt(int64(value), 10), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: %s out of range (%w)", ErrInvalidArgument, field, err)
	}
	return int32(parsed), nil
}

func mustUint64FromInt64(value int64, field string) uint64 {
	parsed, err := uint64FromInt64(value, field)
	if err != nil {
		panic(err)
	}
	return parsed
}

func toRun(row sqlc.OrchestrationRun) Run {
	return Run{
		ID:                     row.ID.String(),
		TenantID:               row.TenantID,
		OwnerSubject:           row.OwnerSubject,
		LifecycleStatus:        row.LifecycleStatus,
		PlanningStatus:         row.PlanningStatus,
		StatusVersion:          mustUint64FromInt64(row.StatusVersion, "run.status_version"),
		PlannerEpoch:           mustUint64FromInt64(row.PlannerEpoch, "run.planner_epoch"),
		RootTaskID:             row.RootTaskID.String(),
		Goal:                   row.Goal,
		Input:                  decodeJSONObject(row.Input),
		OutputSchema:           decodeJSONObject(row.OutputSchema),
		RequestedControlPolicy: decodeJSONObject(row.RequestedControlPolicy),
		ControlPolicy:          decodeJSONObject(row.ControlPolicy),
		SourceMetadata:         decodeJSONObject(row.SourceMetadata),
		Policies:               decodeJSONObject(row.Policies),
		CreatedBy:              row.CreatedBy,
		TerminalReason:         row.TerminalReason,
		CreatedAt:              db.TimeFromPg(row.CreatedAt),
		UpdatedAt:              db.TimeFromPg(row.UpdatedAt),
		FinishedAt:             db.TimeFromPg(row.FinishedAt),
	}
}

func toTask(row sqlc.OrchestrationTask) Task {
	return Task{
		ID:                       row.ID.String(),
		RunID:                    row.RunID.String(),
		DecomposedFromTaskID:     row.DecomposedFromTaskID.String(),
		Kind:                     row.Kind,
		Goal:                     row.Goal,
		Inputs:                   decodeJSONObject(row.Inputs),
		PlannerEpoch:             mustUint64FromInt64(row.PlannerEpoch, "task.planner_epoch"),
		SupersededByPlannerEpoch: mustUint64FromInt64(row.SupersededByPlannerEpoch.Int64, "task.superseded_by_planner_epoch"),
		WorkerProfile:            row.WorkerProfile,
		Priority:                 int(row.Priority),
		RetryPolicy:              decodeJSONObject(row.RetryPolicy),
		VerificationPolicy:       decodeJSONObject(row.VerificationPolicy),
		Status:                   row.Status,
		StatusVersion:            mustUint64FromInt64(row.StatusVersion, "task.status_version"),
		WaitingCheckpointID:      row.WaitingCheckpointID.String(),
		WaitingScope:             row.WaitingScope,
		LatestResultID:           row.LatestResultID.String(),
		ReadyAt:                  db.TimeFromPg(row.ReadyAt),
		BlockedReason:            row.BlockedReason,
		TerminalReason:           row.TerminalReason,
		BlackboardScope:          row.BlackboardScope,
		CreatedAt:                db.TimeFromPg(row.CreatedAt),
		UpdatedAt:                db.TimeFromPg(row.UpdatedAt),
	}
}

func toHumanCheckpoint(row sqlc.OrchestrationHumanCheckpoint) HumanCheckpoint {
	options, _ := decodeCheckpointOptions(row.Options)
	defaultAction, _ := decodeCheckpointDefaultAction(row.DefaultAction)
	resumePolicy, _ := decodeCheckpointResumePolicy(row.ResumePolicy)
	return HumanCheckpoint{
		ID:                       row.ID.String(),
		RunID:                    row.RunID.String(),
		TaskID:                   row.TaskID.String(),
		BlocksRun:                row.BlocksRun,
		PlannerEpoch:             mustUint64FromInt64(row.PlannerEpoch, "checkpoint.planner_epoch"),
		SupersededByPlannerEpoch: mustUint64FromInt64(row.SupersededByPlannerEpoch.Int64, "checkpoint.superseded_by_planner_epoch"),
		Status:                   row.Status,
		StatusVersion:            mustUint64FromInt64(row.StatusVersion, "checkpoint.status_version"),
		Question:                 row.Question,
		Options:                  options,
		DefaultAction:            defaultAction,
		ResumePolicy:             resumePolicy,
		TimeoutAt:                db.TimeFromPg(row.TimeoutAt),
		ResolvedBy:               row.ResolvedBy,
		ResolvedMode:             row.ResolvedMode,
		ResolvedOptionID:         row.ResolvedOptionID,
		ResolvedFreeformInput:    row.ResolvedFreeformInput,
		ResolvedAt:               db.TimeFromPg(row.ResolvedAt),
		Metadata:                 decodeJSONObject(row.Metadata),
		CreatedAt:                db.TimeFromPg(row.CreatedAt),
		UpdatedAt:                db.TimeFromPg(row.UpdatedAt),
	}
}

func toArtifact(row sqlc.OrchestrationArtifact) Artifact {
	return Artifact{
		ID:          row.ID.String(),
		RunID:       row.RunID.String(),
		TaskID:      row.TaskID.String(),
		AttemptID:   row.AttemptID.String(),
		Kind:        row.Kind,
		URI:         row.Uri,
		Version:     row.Version,
		Digest:      row.Digest,
		ContentType: row.ContentType,
		Summary:     row.Summary,
		Metadata:    decodeJSONObject(row.Metadata),
		CreatedAt:   db.TimeFromPg(row.CreatedAt),
	}
}

func toRunEvent(row sqlc.OrchestrationEvent) RunEvent {
	return RunEvent{
		ID:               row.ID.String(),
		RunID:            row.RunID.String(),
		TaskID:           row.TaskID.String(),
		AttemptID:        row.AttemptID.String(),
		CheckpointID:     row.CheckpointID.String(),
		Seq:              mustUint64FromInt64(row.Seq, "event.seq"),
		AggregateType:    row.AggregateType,
		AggregateID:      row.AggregateID.String(),
		AggregateVersion: mustUint64FromInt64(row.AggregateVersion, "event.aggregate_version"),
		Type:             row.Type,
		CausationEventID: row.CausationEventID.String(),
		CorrelationID:    row.CorrelationID,
		IdempotencyKey:   row.IdempotencyKey,
		Payload:          decodeJSONObject(row.Payload),
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		PublishedAt:      db.TimeFromPg(row.PublishedAt),
	}
}
