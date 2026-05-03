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
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type Service struct {
	pool               *pgxpool.Pool
	queries            *sqlc.Queries
	logger             *slog.Logger
	attemptAssignments AttemptAssignmentPublisher
	startRunPlanner    StartRunPlanner
	replanner          Replanner
}

type StartRunPlanner interface {
	PlanStartRun(context.Context, StartRunPlanningInput) (*StartRunPlanningResult, error)
}

type Replanner interface {
	PlanReplan(context.Context, ReplanPlanningInput) (*ReplanPlanningResult, error)
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

func (s *Service) SetStartRunPlanner(planner StartRunPlanner) {
	s.startRunPlanner = planner
}

func (s *Service) SetReplanner(planner Replanner) {
	s.replanner = planner
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
	controlPolicy, err := buildControlPolicy(caller, req.RequestedControlPolicy)
	if err != nil {
		return RunHandle{}, err
	}
	sourceMetadata, err := normalizeStartRunSourceMetadata(req)
	if err != nil {
		return RunHandle{}, err
	}
	req.SourceMetadata = sourceMetadata

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
		ControlPolicy:          marshalJSON(controlPolicy),
		SourceMetadata:         marshalObject(sourceMetadata),
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
		WorkerProfile:        rootWorkerProfile(req.Input, sourceMetadata),
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

func (s *Service) ListBotRuns(ctx context.Context, caller ControlIdentity, botID string, req ListBotRunsRequest) (*RunListPage, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	normalizedBotID, err := normalizeRequiredUUID(botID, "bot id")
	if err != nil {
		return nil, err
	}
	limitCount, err := int32FromInt(normalizedListLimit(req.Limit), "run list limit")
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListOrchestrationRunsByBot(ctx, sqlc.ListOrchestrationRunsByBotParams{
		TenantID:     caller.TenantID,
		OwnerSubject: caller.Subject,
		BotID:        []byte(normalizedBotID),
		LimitCount:   limitCount,
	})
	if err != nil {
		return nil, fmt.Errorf("list orchestration runs by bot: %w", err)
	}
	items := make([]RunListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toRunListItem(row))
	}
	return &RunListPage{Items: items}, nil
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

func (s *Service) GetRunInspector(ctx context.Context, caller ControlIdentity, runID string) (*RunInspector, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	pgRunID, err := db.ParseUUID(runID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run id", ErrInvalidArgument)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("begin run inspector tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	row, err := qtx.GetOrchestrationRunByID(ctx, pgRunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("get orchestration run: %w", err)
	}
	if err := authorizeRun(caller, row); err != nil {
		return nil, ErrRunNotFound
	}
	currentSeq, err := uint64FromInt64(row.LastEventSeq, "run last_event_seq")
	if err != nil {
		return nil, err
	}
	runSnapshot := toRun(row)

	taskRows, err := qtx.ListCurrentOrchestrationTasksByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list current orchestration tasks: %w", err)
	}
	tasks := make([]Task, 0, len(taskRows))
	for _, taskRow := range taskRows {
		tasks = append(tasks, toTask(taskRow))
	}

	checkpointRows, err := qtx.ListCurrentOrchestrationCheckpointsByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list current orchestration checkpoints: %w", err)
	}
	checkpoints := make([]HumanCheckpoint, 0, len(checkpointRows))
	for _, checkpointRow := range checkpointRows {
		checkpoints = append(checkpoints, toHumanCheckpoint(checkpointRow))
	}

	artifactRows, err := qtx.ListCurrentOrchestrationArtifactsByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list current orchestration artifacts: %w", err)
	}
	artifacts := make([]Artifact, 0, len(artifactRows))
	for _, artifactRow := range artifactRows {
		artifacts = append(artifacts, toArtifact(artifactRow))
	}

	dependencyRows, err := qtx.ListCurrentOrchestrationTaskDependenciesByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list orchestration task dependencies: %w", err)
	}
	dependencies := make([]TaskDependency, 0, len(dependencyRows))
	for _, dependencyRow := range dependencyRows {
		dependencies = append(dependencies, toTaskDependency(dependencyRow))
	}

	resultRows, err := qtx.ListCurrentOrchestrationTaskResultsByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list orchestration task results: %w", err)
	}
	results := make([]TaskResult, 0, len(resultRows))
	for _, resultRow := range resultRows {
		results = append(results, toTaskResult(resultRow))
	}

	attemptRows, err := qtx.ListCurrentOrchestrationTaskAttemptsByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list orchestration task attempts: %w", err)
	}
	attempts := make([]InspectorAttempt, 0, len(attemptRows))
	workerIDs := make(map[string]struct{})
	for _, attemptRow := range attemptRows {
		attempts = append(attempts, toInspectorAttempt(attemptRow))
		if workerID := strings.TrimSpace(attemptRow.WorkerID); workerID != "" {
			workerIDs[workerID] = struct{}{}
		}
	}

	inputManifests := make([]InputManifest, 0)
	if len(attemptRows) > 0 {
		manifestIDs := make([]string, 0, len(attemptRows))
		seenManifestIDs := make(map[string]struct{}, len(attemptRows))
		for _, attemptRow := range attemptRows {
			manifestID := attemptRow.InputManifestID.String()
			if manifestID == "" {
				continue
			}
			if _, seen := seenManifestIDs[manifestID]; seen {
				continue
			}
			seenManifestIDs[manifestID] = struct{}{}
			manifestIDs = append(manifestIDs, manifestID)
		}
		sort.Strings(manifestIDs)
		inputManifests = make([]InputManifest, 0, len(manifestIDs))
		for _, manifestID := range manifestIDs {
			pgManifestID, parseErr := db.ParseUUID(manifestID)
			if parseErr != nil {
				return nil, fmt.Errorf("parse orchestration input manifest id: %w", parseErr)
			}
			manifestRow, loadErr := qtx.GetOrchestrationInputManifestByID(ctx, pgManifestID)
			if loadErr != nil {
				return nil, fmt.Errorf("get orchestration input manifest by id: %w", loadErr)
			}
			inputManifests = append(inputManifests, toInputManifest(manifestRow))
		}
	}

	verificationRows, err := qtx.ListCurrentOrchestrationTaskVerificationsByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list orchestration task verifications: %w", err)
	}
	verifications := make([]InspectorVerification, 0, len(verificationRows))
	for _, verificationRow := range verificationRows {
		verifications = append(verifications, toInspectorVerification(verificationRow))
		if workerID := strings.TrimSpace(verificationRow.WorkerID); workerID != "" {
			workerIDs[workerID] = struct{}{}
		}
	}

	actionRecordRows, err := qtx.ListCurrentOrchestrationActionRecordsByRun(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list orchestration action records: %w", err)
	}
	actionRecords := make([]ActionRecord, 0, len(actionRecordRows))
	for _, actionRecordRow := range actionRecordRows {
		actionRecords = append(actionRecords, toActionRecord(actionRecordRow))
	}

	workers := make([]InspectorWorkerLease, 0, len(workerIDs))
	if len(workerIDs) > 0 {
		ids := make([]string, 0, len(workerIDs))
		for workerID := range workerIDs {
			ids = append(ids, workerID)
		}
		sort.Strings(ids)
		workerRows, loadErr := qtx.ListOrchestrationWorkersByIDs(ctx, ids)
		if loadErr != nil {
			return nil, fmt.Errorf("list orchestration workers: %w", loadErr)
		}
		workers = make([]InspectorWorkerLease, 0, len(workerRows))
		for _, workerRow := range workerRows {
			workers = append(workers, toInspectorWorkerLease(workerRow))
		}
	}
	observedAt, err := databaseNow(ctx, tx)
	if err != nil {
		return nil, err
	}

	afterSeq := uint64(0)
	if currentSeq > uint64(maxEventLimit) {
		afterSeq = currentSeq - uint64(maxEventLimit)
	}
	timelineAfterSeq, err := int64FromUint64(afterSeq, "timeline after_seq")
	if err != nil {
		return nil, err
	}
	timelineUntilSeq, err := int64FromUint64(currentSeq, "timeline until_seq")
	if err != nil {
		return nil, err
	}
	eventRows, err := qtx.ListOrchestrationRunEvents(ctx, sqlc.ListOrchestrationRunEventsParams{
		RunID:      row.ID,
		AfterSeq:   timelineAfterSeq,
		UntilSeq:   timelineUntilSeq,
		LimitCount: int32(maxEventLimit),
	})
	if err != nil {
		return nil, fmt.Errorf("list orchestration timeline events: %w", err)
	}
	timeline := make([]RunTimelineEntry, 0, len(eventRows))
	for _, eventRow := range eventRows {
		timeline = append(timeline, toRunTimelineEntry(eventRow))
	}

	executionSpans := buildRunExecutionSpans(attemptRows, verificationRows, resultRows, eventRows)
	stuckSignals := buildRunStuckSignals(tasks, attempts, verifications, workers, observedAt.UTC())
	summary := summarizeRunInspector(tasks, checkpoints, attempts, verifications, workers, stuckSignals)

	return &RunInspector{
		Run:            runSnapshot,
		Summary:        summary,
		StuckSignals:   stuckSignals,
		Tasks:          tasks,
		Dependencies:   dependencies,
		Results:        results,
		Attempts:       attempts,
		Verifications:  verifications,
		InputManifests: inputManifests,
		ExecutionSpans: executionSpans,
		ActionRecords:  actionRecords,
		Checkpoints:    checkpoints,
		Artifacts:      artifacts,
		Workers:        workers,
		Timeline:       timeline,
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
	items = filterCurrentTasks(items)
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

func (s *Service) WatchRun(ctx context.Context, caller ControlIdentity, runID string, req WatchRunRequest) (<-chan RunEvent, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.GetRunSnapshot(ctx, caller, runID)
	if err != nil {
		return nil, err
	}
	if req.AfterSeq > snapshot.SnapshotSeq {
		return nil, fmt.Errorf("%w: after_seq exceeds current snapshot", ErrInvalidArgument)
	}

	events := make(chan RunEvent, 128)
	go func() {
		defer close(events)

		afterSeq := req.AfterSeq
		flushUntil := func(targetSeq uint64) bool {
			for afterSeq < targetSeq {
				page, listErr := s.ListRunEvents(ctx, caller, runID, ListRunEventsRequest{
					AfterSeq: afterSeq,
					UntilSeq: targetSeq,
					Limit:    defaultEventLimit,
				})
				if listErr != nil {
					s.logger.Warn("watch run list events failed", slog.String("run_id", runID), slog.Any("error", listErr))
					return false
				}
				if len(page.Items) == 0 {
					return true
				}
				for _, event := range page.Items {
					select {
					case <-ctx.Done():
						return false
					case events <- event:
					}
					afterSeq = event.Seq
				}
			}
			return true
		}

		if !flushUntil(snapshot.SnapshotSeq) {
			return
		}

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentSnapshot, snapshotErr := s.GetRunSnapshot(ctx, caller, runID)
				if snapshotErr != nil {
					s.logger.Warn("watch run refresh failed", slog.String("run_id", runID), slog.Any("error", snapshotErr))
					return
				}
				if currentSnapshot.SnapshotSeq <= afterSeq {
					continue
				}
				if !flushUntil(currentSnapshot.SnapshotSeq) {
					return
				}
			}
		}
	}()

	return events, nil
}

func (s *Service) InjectRunHint(ctx context.Context, caller ControlIdentity, runID string, req InjectRunHintRequest) (*InjectRunHintResult, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	normalizedRunID, err := normalizeRequiredUUID(runID, "run_id")
	if err != nil {
		return nil, err
	}
	normalizedIdempotencyKey := normalizeIdempotencyKey(req.IdempotencyKey)
	if normalizedIdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidArgument)
	}
	normalizedHint, targetTaskID, requestHash, err := normalizeInjectRunHintRequest(req, normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}

	pgRunID, err := db.ParseUUID(normalizedRunID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run_id", ErrInvalidArgument)
	}
	pgTaskID := pgtype.UUID{}
	if strings.TrimSpace(targetTaskID) != "" {
		pgTaskID, err = db.ParseUUID(targetTaskID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid target_task_id", ErrInvalidArgument)
		}
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin inject run hint tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	existing, existingFound, err := getIdempotencyRecord(ctx, qtx, caller, methodInjectRunHint, normalizedRunID, normalizedIdempotencyKey)
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
		result, err := decodeInjectRunHintResult(existing.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed inject run hint tx: %w", err)
		}
		return &result, nil
	}

	lockedRun, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, pgRunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for inject hint: %w", err)
	}
	if err := authorizeRun(caller, lockedRun); err != nil {
		return nil, ErrRunNotFound
	}
	if !runAcceptsExternalMutations(lockedRun.LifecycleStatus) {
		return nil, ErrRunImmutable
	}

	var lockedTask sqlc.OrchestrationTask
	if pgTaskID.Valid {
		lockedTask, err = qtx.GetOrchestrationTaskByIDForUpdate(ctx, pgTaskID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrTaskNotFound
			}
			return nil, fmt.Errorf("lock target task for inject hint: %w", err)
		}
		if lockedTask.RunID != lockedRun.ID {
			return nil, ErrTaskNotFound
		}
		if taskSuperseded(lockedTask) {
			return nil, ErrTaskImmutable
		}
		if normalizedHint.Kind == RunHintKindConstraintUpdate && !taskAcceptsConstraintUpdate(lockedTask.Status) {
			return nil, ErrRunHintUnsupported
		}
	}
	if normalizedHint.Kind == RunHintKindReplanRequest && !runHintHasReplacementPlan(normalizedHint) {
		if s.replanner == nil {
			return nil, fmt.Errorf("%w: replanner is not configured", ErrRunHintUnsupported)
		}
		sourceMetadata := decodeJSONObject(lockedRun.SourceMetadata)
		if strings.TrimSpace(stringValue(sourceMetadata["bot_id"])) == "" {
			return nil, fmt.Errorf("%w: replanner requires bot_id for replan_request without replacement_plan", ErrRunHintUnsupported)
		}
	}

	record, replay, err := ensureIdempotencyRecord(ctx, qtx, caller, methodInjectRunHint, normalizedRunID, normalizedIdempotencyKey, requestHash)
	if err != nil {
		return nil, err
	}
	if replay {
		result, err := decodeInjectRunHintResult(record.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed inject run hint tx: %w", err)
		}
		return &result, nil
	}

	var (
		lastEvent        sqlc.OrchestrationEvent
		planningIntentID string
	)
	switch normalizedHint.Kind {
	case RunHintKindReplanRequest:
		allTasks, listTasksErr := qtx.ListCurrentOrchestrationTasksByRun(ctx, lockedRun.ID)
		if listTasksErr != nil {
			return nil, fmt.Errorf("list current tasks for replan hint: %w", listTasksErr)
		}
		subtreeTaskIDs := buildTaskSubtreeSet(allTasks, lockedTask.ID)
		allDependencies, listDepsErr := qtx.ListCurrentOrchestrationTaskDependenciesByRun(ctx, lockedRun.ID)
		if listDepsErr != nil {
			return nil, fmt.Errorf("list current task dependencies for replan hint: %w", listDepsErr)
		}
		if err := validateReplanSubtreeIsQuiescent(ctx, qtx, lockedRun.ID, subtreeTaskIDs, allTasks, allDependencies); err != nil {
			return nil, errors.Join(ErrRunHintUnsupported, err)
		}
		planningIntentID, lastEvent, err = s.injectReplanHint(ctx, qtx, lockedRun, lockedTask, normalizedHint, normalizedIdempotencyKey)
		if err != nil {
			return nil, err
		}
		if err := s.syncRunPlanningStatus(ctx, qtx, lockedRun.ID); err != nil {
			return nil, err
		}
	case RunHintKindContextUpdate:
		updatedInput := mergeObjects(decodeJSONObject(lockedRun.Input), normalizedHint.Details)
		updatedRun, updateErr := qtx.UpdateOrchestrationRunInput(ctx, sqlc.UpdateOrchestrationRunInputParams{
			ID:    lockedRun.ID,
			Input: marshalObject(updatedInput),
		})
		if updateErr != nil {
			return nil, fmt.Errorf("update run input for context hint: %w", updateErr)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
			AggregateType:    "run",
			AggregateID:      updatedRun.ID,
			AggregateVersion: updatedRun.StatusVersion,
			IdempotencyKey:   normalizedIdempotencyKey,
			Type:             "run.event.hint.injected",
			Payload: map[string]any{
				"run_id":      updatedRun.ID.String(),
				"hint_kind":   normalizedHint.Kind,
				"summary":     normalizedHint.Summary,
				"details":     normalizedHint.Details,
				"target_kind": "run",
			},
		})
		if err != nil {
			return nil, err
		}
	case RunHintKindConstraintUpdate:
		if pgTaskID.Valid {
			updatedInputs := mergeObjects(decodeJSONObject(lockedTask.Inputs), normalizedHint.Details)
			updatedTask, updateErr := qtx.UpdateOrchestrationTaskInputs(ctx, sqlc.UpdateOrchestrationTaskInputsParams{
				ID:     lockedTask.ID,
				Inputs: marshalObject(updatedInputs),
			})
			if updateErr != nil {
				return nil, fmt.Errorf("update task inputs for constraint hint: %w", updateErr)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
				TaskID:           updatedTask.ID,
				AggregateType:    "task",
				AggregateID:      updatedTask.ID,
				AggregateVersion: updatedTask.StatusVersion,
				IdempotencyKey:   normalizedIdempotencyKey,
				Type:             "run.event.hint.injected",
				Payload: map[string]any{
					"run_id":         lockedRun.ID.String(),
					"task_id":        updatedTask.ID.String(),
					"hint_kind":      normalizedHint.Kind,
					"summary":        normalizedHint.Summary,
					"details":        normalizedHint.Details,
					"target_kind":    "task",
					"target_task_id": updatedTask.ID.String(),
				},
			})
			if err != nil {
				return nil, err
			}
		} else {
			updatedInput := mergeObjects(decodeJSONObject(lockedRun.Input), normalizedHint.Details)
			updatedRun, updateErr := qtx.UpdateOrchestrationRunInput(ctx, sqlc.UpdateOrchestrationRunInputParams{
				ID:    lockedRun.ID,
				Input: marshalObject(updatedInput),
			})
			if updateErr != nil {
				return nil, fmt.Errorf("update run input for constraint hint: %w", updateErr)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
				AggregateType:    "run",
				AggregateID:      updatedRun.ID,
				AggregateVersion: updatedRun.StatusVersion,
				IdempotencyKey:   normalizedIdempotencyKey,
				Type:             "run.event.hint.injected",
				Payload: map[string]any{
					"run_id":      updatedRun.ID.String(),
					"hint_kind":   normalizedHint.Kind,
					"summary":     normalizedHint.Summary,
					"details":     normalizedHint.Details,
					"target_kind": "run",
				},
			})
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("%w: unsupported hint kind %q", ErrInvalidArgument, normalizedHint.Kind)
	}

	result := InjectRunHintResult{
		RunID:            normalizedRunID,
		PlanningIntentID: planningIntentID,
		SnapshotSeq:      mustUint64FromInt64(lastEvent.Seq, "inject_run_hint.event_seq"),
	}
	if _, err := qtx.CompleteOrchestrationIdempotencyRecord(ctx, sqlc.CompleteOrchestrationIdempotencyRecordParams{
		ResponsePayload: marshalJSON(result),
		TenantID:        caller.TenantID,
		CallerSubject:   caller.Subject,
		Method:          methodInjectRunHint,
		TargetID:        normalizedRunID,
		IdempotencyKey:  normalizedIdempotencyKey,
	}); err != nil {
		return nil, fmt.Errorf("complete inject run hint idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit inject run hint tx: %w", err)
	}
	return &result, nil
}

func (s *Service) RetryTask(ctx context.Context, caller ControlIdentity, taskID string, req RetryTaskRequest) (*RetryTaskResult, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	normalizedTaskID, err := normalizeRequiredUUID(taskID, "task_id")
	if err != nil {
		return nil, err
	}
	normalizedIdempotencyKey := normalizeIdempotencyKey(req.IdempotencyKey)
	if normalizedIdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidArgument)
	}
	requestHash, err := retryTaskRequestHash(normalizedTaskID, req, normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}
	pgTaskID, err := db.ParseUUID(normalizedTaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid task_id", ErrInvalidArgument)
	}
	normalizedExpectedRunID := strings.TrimSpace(req.ExpectedRunID)
	var pgExpectedRunID pgtype.UUID
	if normalizedExpectedRunID != "" {
		normalizedExpectedRunID, err = normalizeRequiredUUID(normalizedExpectedRunID, "run_id")
		if err != nil {
			return nil, err
		}
		pgExpectedRunID, err = db.ParseUUID(normalizedExpectedRunID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid run_id", ErrInvalidArgument)
		}
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin retry task tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	existing, existingFound, err := getIdempotencyRecord(ctx, qtx, caller, methodRetryTask, normalizedTaskID, normalizedIdempotencyKey)
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
		result, err := decodeRetryTaskResult(existing.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed retry task tx: %w", err)
		}
		return &result, nil
	}

	taskPreview, err := qtx.GetOrchestrationTaskByID(ctx, pgTaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("load task for retry: %w", err)
	}
	if pgExpectedRunID.Valid && taskPreview.RunID != pgExpectedRunID {
		return nil, ErrTaskNotFound
	}
	lockedRun, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, taskPreview.RunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for retry: %w", err)
	}
	lockedTask, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, pgTaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock task for retry: %w", err)
	}
	if lockedTask.RunID != lockedRun.ID {
		return nil, ErrTaskNotFound
	}
	if err := authorizeRun(caller, lockedRun); err != nil {
		return nil, ErrTaskNotFound
	}
	if !runAcceptsRetry(lockedRun.LifecycleStatus) {
		return nil, ErrRunImmutable
	}
	if taskSuperseded(lockedTask) || lockedTask.Status != TaskStatusFailed {
		return nil, ErrTaskRetryUnsupported
	}

	activeAttempts, err := qtx.CountActiveOrchestrationTaskAttemptsByTask(ctx, lockedTask.ID)
	if err != nil {
		return nil, fmt.Errorf("count active task attempts for retry: %w", err)
	}
	if activeAttempts > 0 {
		return nil, ErrTaskRetryUnsupported
	}
	activePlanningIntents, err := qtx.CountActiveOrchestrationPlanningIntentsByRun(ctx, lockedRun.ID)
	if err != nil {
		return nil, fmt.Errorf("count active planning intents for retry: %w", err)
	}
	if activePlanningIntents > 0 {
		return nil, ErrTaskRetryUnsupported
	}
	runAttempts, err := qtx.ListCurrentOrchestrationTaskAttemptsByRun(ctx, lockedRun.ID)
	if err != nil {
		return nil, fmt.Errorf("list run attempts for retry: %w", err)
	}
	for _, attempt := range runAttempts {
		if attempt.TaskID != lockedTask.ID && isActiveAttemptStatus(attempt.Status) {
			return nil, ErrTaskRetryUnsupported
		}
	}
	runVerifications, err := qtx.ListCurrentOrchestrationTaskVerificationsByRun(ctx, lockedRun.ID)
	if err != nil {
		return nil, fmt.Errorf("list task verifications for retry: %w", err)
	}
	for _, verification := range runVerifications {
		if verification.TaskID == lockedTask.ID || isActiveVerificationStatus(verification.Status) {
			return nil, ErrTaskRetryUnsupported
		}
	}

	record, replay, err := ensureIdempotencyRecord(ctx, qtx, caller, methodRetryTask, normalizedTaskID, normalizedIdempotencyKey, requestHash)
	if err != nil {
		return nil, err
	}
	if replay {
		result, err := decodeRetryTaskResult(record.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed retry task tx: %w", err)
		}
		return &result, nil
	}

	readyTask, err := qtx.MarkOrchestrationTaskReadyForRetry(ctx, lockedTask.ID)
	if err != nil {
		return nil, fmt.Errorf("mark task ready for retry: %w", err)
	}
	if lockedRun.LifecycleStatus == LifecycleStatusFailed {
		runningRun, markRunErr := qtx.MarkOrchestrationRunRunning(ctx, lockedRun.ID)
		if markRunErr != nil {
			return nil, fmt.Errorf("mark failed run running for retry: %w", markRunErr)
		}
		if _, err = s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
			TaskID:           readyTask.ID,
			AggregateType:    "run",
			AggregateID:      runningRun.ID,
			AggregateVersion: runningRun.StatusVersion,
			IdempotencyKey:   normalizedIdempotencyKey,
			Type:             "run.event.running",
			Payload: map[string]any{
				"run_id":          runningRun.ID.String(),
				"previous_status": lockedRun.LifecycleStatus,
				"new_status":      runningRun.LifecycleStatus,
				"entry_reason":    "retry_task",
				"task_id":         readyTask.ID.String(),
			},
		}); err != nil {
			return nil, err
		}
		lockedRun = runningRun
	}
	lastEvent, err := s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
		TaskID:           readyTask.ID,
		CheckpointID:     pgtype.UUID{},
		AggregateType:    "task",
		AggregateID:      readyTask.ID,
		AggregateVersion: readyTask.StatusVersion,
		IdempotencyKey:   normalizedIdempotencyKey,
		Type:             "run.event.task.ready",
		Payload: map[string]any{
			"task_id":         readyTask.ID.String(),
			"previous_status": lockedTask.Status,
			"new_status":      readyTask.Status,
			"ready_reason":    "retry_task",
			"retry_reason":    strings.TrimSpace(req.Reason),
		},
	})
	if err != nil {
		return nil, err
	}

	result := RetryTaskResult{
		TaskID:      normalizedTaskID,
		RunID:       lockedRun.ID.String(),
		SnapshotSeq: mustUint64FromInt64(lastEvent.Seq, "retry_task.event_seq"),
	}
	if _, err := qtx.CompleteOrchestrationIdempotencyRecord(ctx, sqlc.CompleteOrchestrationIdempotencyRecordParams{
		ResponsePayload: marshalJSON(result),
		TenantID:        caller.TenantID,
		CallerSubject:   caller.Subject,
		Method:          methodRetryTask,
		TargetID:        normalizedTaskID,
		IdempotencyKey:  normalizedIdempotencyKey,
	}); err != nil {
		return nil, fmt.Errorf("complete retry task idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit retry task tx: %w", err)
	}
	return &result, nil
}

func (s *Service) injectReplanHint(
	ctx context.Context,
	qtx *sqlc.Queries,
	lockedRun sqlc.OrchestrationRun,
	lockedTask sqlc.OrchestrationTask,
	normalizedHint RunHint,
	idempotencyKey string,
) (string, sqlc.OrchestrationEvent, error) {
	planningIntentID, planningIntentUUID, err := newPGUUID()
	if err != nil {
		return "", sqlc.OrchestrationEvent{}, err
	}
	payload := map[string]any{
		"run_id":             lockedRun.ID.String(),
		"source_task_id":     lockedTask.ID.String(),
		"reason":             strings.TrimSpace(normalizedHint.Summary),
		"base_planner_epoch": lockedRun.PlannerEpoch,
		"injected_hint": map[string]any{
			"kind":           normalizedHint.Kind,
			"summary":        normalizedHint.Summary,
			"details":        normalizedHint.Details,
			"target_task_id": normalizedHint.TargetTaskID,
		},
	}
	if replacementPlan, ok := normalizedHint.Details["replacement_plan"].(map[string]any); ok {
		payload["replacement_plan"] = normalizeObject(replacementPlan)
	}
	planningIntent, err := qtx.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               planningIntentUUID,
		RunID:            lockedRun.ID,
		TaskID:           lockedTask.ID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindReplan,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: lockedRun.PlannerEpoch,
		Payload:          marshalJSON(payload),
	})
	if err != nil {
		return "", sqlc.OrchestrationEvent{}, fmt.Errorf("create injected run hint planning intent: %w", err)
	}
	lastEvent, err := s.appendEvent(ctx, qtx, lockedRun.ID, eventSpec{
		TaskID:           lockedTask.ID,
		CheckpointID:     pgtype.UUID{},
		AggregateType:    "planning_intent",
		AggregateID:      planningIntent.ID,
		AggregateVersion: 1,
		IdempotencyKey:   idempotencyKey,
		Type:             "run.event.planning_intent.enqueued",
		Payload: map[string]any{
			"planning_intent_id": planningIntentID,
			"run_id":             lockedRun.ID.String(),
			"task_id":            lockedTask.ID.String(),
			"kind":               planningIntent.Kind,
			"status":             planningIntent.Status,
			"reason":             strings.TrimSpace(normalizedHint.Summary),
			"hint_kind":          normalizedHint.Kind,
		},
	})
	if err != nil {
		return "", sqlc.OrchestrationEvent{}, err
	}
	return planningIntentID, lastEvent, nil
}

func (s *Service) CancelRun(ctx context.Context, caller ControlIdentity, runID string, req CancelRunRequest) (*CancelRunResult, error) {
	var err error
	caller, err = normalizeControlIdentity(caller)
	if err != nil {
		return nil, err
	}
	normalizedIdempotencyKey := normalizeIdempotencyKey(req.IdempotencyKey)
	if normalizedIdempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required", ErrInvalidArgument)
	}
	pgRunID, err := db.ParseUUID(runID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid run id", ErrInvalidArgument)
	}
	normalizedRunID := pgRunID.String()
	hash, err := cancelRunRequestHash(normalizedRunID, normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin cancel run tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	existing, existingFound, err := getIdempotencyRecord(ctx, qtx, caller, methodCancelRun, normalizedRunID, normalizedIdempotencyKey)
	if err != nil {
		return nil, err
	}

	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, pgRunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for cancel: %w", err)
	}
	if err := authorizeRun(caller, runRow); err != nil {
		return nil, ErrRunNotFound
	}
	if existingFound {
		if existing.RequestHash != hash {
			return nil, ErrIdempotencyConflict
		}
		if existing.State != "completed" {
			return nil, ErrIdempotencyIncomplete
		}
		result, err := decodeCancelRunResult(existing.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed cancel run tx: %w", err)
		}
		return &result, nil
	}

	record, replay, err := ensureIdempotencyRecord(ctx, qtx, caller, methodCancelRun, normalizedRunID, normalizedIdempotencyKey, hash)
	if err != nil {
		return nil, err
	}
	if replay {
		result, err := decodeCancelRunResult(record.ResponsePayload)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed cancel run tx: %w", err)
		}
		return &result, nil
	}

	switch runRow.LifecycleStatus {
	case LifecycleStatusCompleted, LifecycleStatusFailed, LifecycleStatusCancelled:
		return nil, ErrRunImmutable
	}

	lastEvent := sqlc.OrchestrationEvent{}
	if runRow.LifecycleStatus != LifecycleStatusCancelling {
		cancellingRun, err := qtx.MarkOrchestrationRunCancelling(ctx, runRow.ID)
		if err != nil {
			return nil, fmt.Errorf("mark run cancelling: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			AggregateType:    "run",
			AggregateID:      cancellingRun.ID,
			AggregateVersion: cancellingRun.StatusVersion,
			Type:             "run.event.cancelling",
			IdempotencyKey:   normalizedIdempotencyKey,
			Payload: map[string]any{
				"run_id":          cancellingRun.ID.String(),
				"previous_status": runRow.LifecycleStatus,
				"new_status":      cancellingRun.LifecycleStatus,
			},
		})
		if err != nil {
			return nil, err
		}
		runRow = cancellingRun
	}

	checkpoints, err := qtx.ListCurrentOrchestrationCheckpointsByRun(ctx, runRow.ID)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints for cancel: %w", err)
	}
	for _, checkpoint := range checkpoints {
		if checkpoint.Status != CheckpointStatusOpen {
			continue
		}
		cancelledCheckpoint, err := qtx.MarkOrchestrationHumanCheckpointCancelled(ctx, checkpoint.ID)
		if err != nil {
			return nil, fmt.Errorf("cancel open checkpoint: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           cancelledCheckpoint.TaskID,
			CheckpointID:     cancelledCheckpoint.ID,
			AggregateType:    "checkpoint",
			AggregateID:      cancelledCheckpoint.ID,
			AggregateVersion: cancelledCheckpoint.StatusVersion,
			Type:             "run.event.hitl.cancelled",
			IdempotencyKey:   normalizedIdempotencyKey,
			Payload: map[string]any{
				"checkpoint_id":   cancelledCheckpoint.ID.String(),
				"task_id":         cancelledCheckpoint.TaskID.String(),
				"previous_status": checkpoint.Status,
				"new_status":      cancelledCheckpoint.Status,
				"status":          cancelledCheckpoint.Status,
				"blocks_run":      cancelledCheckpoint.BlocksRun,
			},
		})
		if err != nil {
			return nil, err
		}
	}

	tasks, err := qtx.ListCurrentOrchestrationTasksByRun(ctx, runRow.ID)
	if err != nil {
		return nil, fmt.Errorf("list tasks for cancel: %w", err)
	}
	for _, task := range tasks {
		if taskSuperseded(task) {
			continue
		}
		switch task.Status {
		case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
			continue
		}
		if err := s.cancelTaskDuringRunCancellation(ctx, qtx, runRow, task, pgtype.UUID{}); err != nil {
			return nil, err
		}
	}

	runRow, err = qtx.GetOrchestrationRunByIDForUpdate(ctx, runRow.ID)
	if err != nil {
		return nil, fmt.Errorf("lock run after cancel mutations: %w", err)
	}
	if terminalEvent, terminal, err := s.maybeMarkRunCompletedAfterTaskTransition(ctx, qtx, runRow, pgtype.UUID{}, pgtype.UUID{}); err != nil {
		return nil, err
	} else if terminal {
		lastEvent = terminalEvent
		runRow, err = qtx.GetOrchestrationRunByIDForUpdate(ctx, runRow.ID)
		if err != nil {
			return nil, fmt.Errorf("lock run after cancel completion: %w", err)
		}
	}
	snapshotSeq, err := uint64FromInt64(runRow.LastEventSeq, "cancel run event seq")
	if err != nil {
		return nil, err
	}
	if snapshotSeq == 0 && lastEvent.Seq > 0 {
		snapshotSeq, err = uint64FromInt64(lastEvent.Seq, "cancel run last event seq")
		if err != nil {
			return nil, err
		}
	}
	result := CancelRunResult{
		RunID:           normalizedRunID,
		LifecycleStatus: runRow.LifecycleStatus,
		SnapshotSeq:     snapshotSeq,
	}
	if _, err := qtx.CompleteOrchestrationIdempotencyRecord(ctx, sqlc.CompleteOrchestrationIdempotencyRecordParams{
		ResponsePayload: marshalJSON(result),
		TenantID:        caller.TenantID,
		CallerSubject:   caller.Subject,
		Method:          methodCancelRun,
		TargetID:        normalizedRunID,
		IdempotencyKey:  normalizedIdempotencyKey,
	}); err != nil {
		return nil, fmt.Errorf("complete cancel run idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cancel run tx: %w", err)
	}
	return &result, nil
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

	if lockedTask.Status != TaskStatusReady {
		runAttempts, err := qtx.ListCurrentOrchestrationTaskAttemptsByRun(ctx, pgRunID)
		if err != nil {
			return nil, fmt.Errorf("list run attempts for checkpoint create: %w", err)
		}
		runVerifications, err := qtx.ListCurrentOrchestrationTaskVerificationsByRun(ctx, pgRunID)
		if err != nil {
			return nil, fmt.Errorf("list run verifications for checkpoint create: %w", err)
		}
		if err := s.pauseRunBarrierSiblingWork(ctx, qtx, pgRunID, checkpointUUID, lockedTask, runAttempts, runVerifications); err != nil {
			if errors.Is(err, ErrRunBarrierUnsupported) {
				return nil, ErrTaskCheckpointUnsupported
			}
			return nil, err
		}
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

func runAcceptsRetry(status string) bool {
	if status == LifecycleStatusFailed {
		return true
	}
	return runAcceptsExternalMutations(status)
}

func runAcceptsPlanningIntent(kind, status string, basePlannerEpoch, currentPlannerEpoch int64) bool {
	if status == LifecycleStatusCancelling {
		switch kind {
		case PlanningIntentKindAttemptFinalize, PlanningIntentKindCheckpointResume:
			return true
		default:
			return false
		}
	}
	if status == LifecycleStatusFailed {
		return kind == PlanningIntentKindAttemptFinalize
	}
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

func normalizeRequiredUUID(raw string, field string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidArgument, field)
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
	switch status {
	case TaskStatusReady, TaskStatusDispatching, TaskStatusRunning, TaskStatusVerifying:
		return true
	default:
		return false
	}
}

func taskAcceptsConstraintUpdate(status string) bool {
	switch status {
	case TaskStatusCreated, TaskStatusReady, TaskStatusBlocked, TaskStatusWaitingHuman, TaskStatusFailed:
		return true
	default:
		return false
	}
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

func verifierProfileForRunAndTask(runRow sqlc.OrchestrationRun, taskRow sqlc.OrchestrationTask, policy map[string]any) string {
	profile := verifierProfileForTaskPolicy(policy)
	if profile != DefaultVerifierProfile {
		return profile
	}
	sourceMetadata := decodeJSONObject(runRow.SourceMetadata)
	if strings.TrimSpace(stringValue(sourceMetadata["bot_id"])) != "" {
		return profile
	}
	if strings.TrimSpace(taskRow.WorkerProfile) == BuiltinEchoWorkerProfile {
		return BuiltinBasicVerifierProfile
	}
	return BuiltinBasicVerifierProfile
}

func filterCurrentTasks(items []Task) []Task {
	if len(items) == 0 {
		return items
	}
	filtered := make([]Task, 0, len(items))
	for _, item := range items {
		if item.SupersededByPlannerEpoch != 0 {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (s *Service) cancelTaskDuringRunCancellation(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, taskRow sqlc.OrchestrationTask, attemptID pgtype.UUID) error {
	if taskRow.Status == TaskStatusCancelled {
		_, _, err := s.maybeMarkRunCompletedAfterTaskTransition(ctx, qtx, runRow, taskRow.ID, attemptID)
		return err
	}
	runAttempts, err := qtx.ListCurrentOrchestrationTaskAttemptsByRun(ctx, runRow.ID)
	if err != nil {
		return fmt.Errorf("list task attempts during run cancellation: %w", err)
	}
	if err := s.cancelTaskAttemptsDuringRunCancellation(ctx, qtx, runRow.ID, taskRow, runAttempts); err != nil {
		return err
	}
	runVerifications, err := qtx.ListCurrentOrchestrationTaskVerificationsByRun(ctx, runRow.ID)
	if err != nil {
		return fmt.Errorf("list task verifications during run cancellation: %w", err)
	}
	if err := s.cancelTaskVerificationsDuringRunCancellation(ctx, qtx, runRow.ID, taskRow, runVerifications); err != nil {
		return err
	}
	cancelledTask, err := qtx.MarkOrchestrationTaskCancelled(ctx, sqlc.MarkOrchestrationTaskCancelledParams{
		ID:             taskRow.ID,
		TerminalReason: "run cancelled",
	})
	if err != nil {
		return fmt.Errorf("cancel task during run cancellation: %w", err)
	}
	payload := map[string]any{
		"task_id":         cancelledTask.ID.String(),
		"previous_status": taskRow.Status,
		"new_status":      cancelledTask.Status,
		"terminal_reason": cancelledTask.TerminalReason,
	}
	if attemptID.Valid {
		payload["attempt_id"] = attemptID.String()
	}
	if _, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		TaskID:           cancelledTask.ID,
		AttemptID:        attemptID,
		AggregateType:    "task",
		AggregateID:      cancelledTask.ID,
		AggregateVersion: cancelledTask.StatusVersion,
		Type:             "run.event.task.cancelled",
		Payload:          payload,
	}); err != nil {
		return err
	}
	if _, _, err := s.maybeMarkRunCompletedAfterTaskTransition(ctx, qtx, runRow, cancelledTask.ID, attemptID); err != nil {
		return err
	}
	return nil
}

func (s *Service) cancelTaskAttemptsDuringRunCancellation(
	ctx context.Context,
	qtx *sqlc.Queries,
	runID pgtype.UUID,
	taskRow sqlc.OrchestrationTask,
	runAttempts []sqlc.OrchestrationTaskAttempt,
) error {
	for _, attempt := range runAttempts {
		if attempt.TaskID != taskRow.ID || isTerminalAttemptStatus(attempt.Status) {
			continue
		}
		lockedAttempt, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, attempt.ID)
		if err != nil {
			return fmt.Errorf("lock task attempt during run cancellation: %w", err)
		}
		if isTerminalAttemptStatus(lockedAttempt.Status) {
			continue
		}
		switch lockedAttempt.Status {
		case TaskAttemptStatusCreated:
			finalAttempt, err := qtx.RetireCreatedOrchestrationTaskAttemptFailed(ctx, sqlc.RetireCreatedOrchestrationTaskAttemptFailedParams{
				ID:             lockedAttempt.ID,
				FailureClass:   "run_cancelled",
				TerminalReason: "run cancelled",
			})
			if err != nil {
				return fmt.Errorf("retire created attempt during run cancellation: %w", err)
			}
			if _, err := s.appendEvent(ctx, qtx, runID, eventSpec{
				TaskID:           finalAttempt.TaskID,
				AttemptID:        finalAttempt.ID,
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
			}); err != nil {
				return err
			}
		case TaskAttemptStatusClaimed, TaskAttemptStatusBinding, TaskAttemptStatusRunning:
			if _, err := s.retireAttempt(ctx, qtx, lockedAttempt, attemptRetirementSpec{
				Status:         TaskAttemptStatusLost,
				FailureClass:   "run_cancelled",
				TerminalReason: "run cancelled",
			}); err != nil {
				return fmt.Errorf("retire leased attempt during run cancellation: %w", err)
			}
		}
	}
	return nil
}

func (s *Service) retireVerificationDuringRunCancellation(
	ctx context.Context,
	qtx *sqlc.Queries,
	runRow sqlc.OrchestrationRun,
	verificationRow sqlc.OrchestrationTaskVerification,
) (sqlc.OrchestrationTaskVerification, error) {
	if isTerminalVerificationStatus(verificationRow.Status) {
		return verificationRow, nil
	}
	cancelledVerification, err := qtx.CancelOrchestrationTaskVerification(ctx, sqlc.CancelOrchestrationTaskVerificationParams{
		ID:             verificationRow.ID,
		Verdict:        VerificationVerdictRejected,
		Summary:        "run cancelled",
		FailureClass:   "run_cancelled",
		TerminalReason: "run cancelled",
	})
	if err != nil {
		return sqlc.OrchestrationTaskVerification{}, fmt.Errorf("cancel verification during run cancellation: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		TaskID:           cancelledVerification.TaskID,
		AggregateType:    "verification",
		AggregateID:      cancelledVerification.ID,
		AggregateVersion: cancelledVerification.ClaimEpoch,
		Type:             "run.event.verification.lost",
		Payload: map[string]any{
			"verification_id": cancelledVerification.ID.String(),
			"task_id":         cancelledVerification.TaskID.String(),
			"previous_status": verificationRow.Status,
			"new_status":      cancelledVerification.Status,
			"terminal_reason": cancelledVerification.TerminalReason,
		},
	}); err != nil {
		return sqlc.OrchestrationTaskVerification{}, err
	}
	return cancelledVerification, nil
}

func (s *Service) cancelTaskVerificationsDuringRunCancellation(
	ctx context.Context,
	qtx *sqlc.Queries,
	runID pgtype.UUID,
	taskRow sqlc.OrchestrationTask,
	runVerifications []sqlc.OrchestrationTaskVerification,
) error {
	for _, verification := range runVerifications {
		if verification.TaskID != taskRow.ID || isTerminalVerificationStatus(verification.Status) {
			continue
		}
		if _, err := s.retireVerificationDuringRunCancellation(ctx, qtx, sqlc.OrchestrationRun{ID: runID}, verification); err != nil {
			return err
		}
	}
	return nil
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

func normalizeInjectRunHintRequest(req InjectRunHintRequest, idempotencyKey string) (RunHint, string, string, error) {
	normalized := RunHint{
		Kind:         strings.TrimSpace(req.Hint.Kind),
		Summary:      strings.TrimSpace(req.Hint.Summary),
		Details:      normalizeObject(req.Hint.Details),
		TargetTaskID: strings.TrimSpace(req.Hint.TargetTaskID),
	}
	switch normalized.Kind {
	case RunHintKindReplanRequest:
		if normalized.TargetTaskID == "" {
			return RunHint{}, "", "", fmt.Errorf("%w: target_task_id is required for replan_request", ErrInvalidArgument)
		}
		if rawReplacementPlan, ok := normalized.Details["replacement_plan"]; ok && rawReplacementPlan != nil {
			replacementPlan, ok := rawReplacementPlan.(map[string]any)
			if !ok {
				return RunHint{}, "", "", fmt.Errorf("%w: replacement_plan must be an object for replan_request", ErrInvalidArgument)
			}
			if err := validateRunHintReplacementPlan(replacementPlan); err != nil {
				return RunHint{}, "", "", err
			}
		}
	case RunHintKindContextUpdate:
		if normalized.TargetTaskID != "" {
			return RunHint{}, "", "", fmt.Errorf("%w: target_task_id must be empty for context_update", ErrInvalidArgument)
		}
		if len(normalized.Details) == 0 {
			return RunHint{}, "", "", fmt.Errorf("%w: details are required for context_update", ErrInvalidArgument)
		}
	case RunHintKindConstraintUpdate:
		if len(normalized.Details) == 0 {
			return RunHint{}, "", "", fmt.Errorf("%w: details are required for constraint_update", ErrInvalidArgument)
		}
	default:
		return RunHint{}, "", "", fmt.Errorf("%w: unsupported hint kind %q", ErrInvalidArgument, normalized.Kind)
	}

	hash, err := hashJSON(struct {
		Hint           RunHint `json:"hint"`
		IdempotencyKey string  `json:"idempotency_key"`
	}{
		Hint:           normalized,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return RunHint{}, "", "", err
	}
	return normalized, normalized.TargetTaskID, hash, nil
}

func runHintHasReplacementPlan(hint RunHint) bool {
	if len(hint.Details) == 0 {
		return false
	}
	rawReplacementPlan, ok := hint.Details["replacement_plan"]
	if !ok || rawReplacementPlan == nil {
		return false
	}
	_, ok = rawReplacementPlan.(map[string]any)
	return ok
}

func validateRunHintReplacementPlan(replacementPlan map[string]any) error {
	rawChildTasks, ok := replacementPlan["child_tasks"]
	if !ok {
		return fmt.Errorf("%w: replacement_plan.child_tasks is required for replan_request", ErrInvalidArgument)
	}
	childTasks, ok := rawChildTasks.([]any)
	if !ok {
		return fmt.Errorf("%w: replacement_plan.child_tasks must be an array for replan_request", ErrInvalidArgument)
	}
	if len(childTasks) == 0 {
		return fmt.Errorf("%w: replacement_plan.child_tasks must not be empty for replan_request", ErrInvalidArgument)
	}
	return nil
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

func cancelRunRequestHash(runID, idempotencyKey string) (string, error) {
	return hashJSON(struct {
		RunID          string `json:"run_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}{
		RunID:          runID,
		IdempotencyKey: idempotencyKey,
	})
}

func retryTaskRequestHash(taskID string, req RetryTaskRequest, idempotencyKey string) (string, error) {
	return hashJSON(struct {
		TaskID         string `json:"task_id"`
		ExpectedRunID  string `json:"expected_run_id"`
		Reason         string `json:"reason"`
		IdempotencyKey string `json:"idempotency_key"`
	}{
		TaskID:         strings.TrimSpace(taskID),
		ExpectedRunID:  strings.TrimSpace(req.ExpectedRunID),
		Reason:         strings.TrimSpace(req.Reason),
		IdempotencyKey: idempotencyKey,
	})
}

func startRunRequestHash(req StartRunRequest) (string, error) {
	sourceMetadata, err := normalizeStartRunSourceMetadata(req)
	if err != nil {
		return "", err
	}
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
		SourceMetadata:         sourceMetadata,
		Policies:               normalizeObject(req.Policies),
	}
	return hashJSON(normalized)
}

func normalizeStartRunSourceMetadata(req StartRunRequest) (map[string]any, error) {
	sourceMetadata := normalizeObject(req.SourceMetadata)
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return sourceMetadata, nil
	}
	existingBotID := strings.TrimSpace(stringValue(sourceMetadata["bot_id"]))
	if existingBotID != "" && existingBotID != botID {
		return nil, fmt.Errorf("%w: bot_id does not match source_metadata.bot_id", ErrInvalidArgument)
	}
	sourceMetadata["bot_id"] = botID
	return sourceMetadata, nil
}

func buildPhase1ControlPolicy(caller ControlIdentity) map[string]any {
	policy, err := buildControlPolicy(caller, nil)
	if err != nil {
		return map[string]any{
			"mode":           ControlPolicyModeOwnerOnly,
			"owner_subject":  strings.TrimSpace(caller.Subject),
			"runtime_limits": defaultRuntimeLimits().Map(),
		}
	}
	return policy
}

func buildControlPolicy(caller ControlIdentity, requested map[string]any) (map[string]any, error) {
	limits, err := runtimeLimitsFromRequestedPolicy(requested)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":           ControlPolicyModeOwnerOnly,
		"owner_subject":  strings.TrimSpace(caller.Subject),
		"runtime_limits": limits.Map(),
	}, nil
}

func rootWorkerProfile(input, sourceMetadata map[string]any) string {
	if _, ok := normalizeObject(input)["builtin_workerd"]; ok {
		return BuiltinEchoWorkerProfile
	}
	if strings.TrimSpace(stringValue(normalizeObject(sourceMetadata)["bot_id"])) == "" {
		return BuiltinEchoWorkerProfile
	}
	return DefaultRootWorkerProfile
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

func decodeCancelRunResult(raw []byte) (CancelRunResult, error) {
	var result CancelRunResult
	if err := unmarshalJSON(raw, &result); err != nil {
		return CancelRunResult{}, fmt.Errorf("decode idempotent cancel run result: %w", err)
	}
	return result, nil
}

func decodeInjectRunHintResult(raw []byte) (InjectRunHintResult, error) {
	var result InjectRunHintResult
	if err := unmarshalJSON(raw, &result); err != nil {
		return InjectRunHintResult{}, fmt.Errorf("decode idempotent inject run hint result: %w", err)
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

func decodeRetryTaskResult(raw []byte) (RetryTaskResult, error) {
	var result RetryTaskResult
	if err := unmarshalJSON(raw, &result); err != nil {
		return RetryTaskResult{}, fmt.Errorf("decode idempotent retry task result: %w", err)
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

func mergeObjects(base, overlay map[string]any) map[string]any {
	merged := normalizeObject(base)
	for key, value := range normalizeObject(overlay) {
		existing, hasExisting := merged[key]
		existingObject, existingIsObject := existing.(map[string]any)
		valueObject, valueIsObject := value.(map[string]any)
		if hasExisting && existingIsObject && valueIsObject {
			merged[key] = mergeObjects(existingObject, valueObject)
			continue
		}
		merged[key] = value
	}
	return merged
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

func uuidToString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
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

func decodeJSONValue(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func decodeJSONArrayObjects(raw []byte) []map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return []map[string]any{}
	}
	var value []map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return []map[string]any{}
	}
	for i := range value {
		value[i] = normalizeObject(value[i])
	}
	return value
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
		FinishedAt:             optionalTime(db.TimeFromPg(row.FinishedAt)),
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
		ReadyAt:                  optionalTime(db.TimeFromPg(row.ReadyAt)),
		BlockedReason:            row.BlockedReason,
		TerminalReason:           row.TerminalReason,
		BlackboardScope:          row.BlackboardScope,
		CreatedAt:                db.TimeFromPg(row.CreatedAt),
		UpdatedAt:                db.TimeFromPg(row.UpdatedAt),
	}
}

func toRunListItem(row sqlc.OrchestrationRun) RunListItem {
	return RunListItem{
		ID:              row.ID.String(),
		Goal:            row.Goal,
		LifecycleStatus: row.LifecycleStatus,
		PlanningStatus:  row.PlanningStatus,
		RootTaskID:      row.RootTaskID.String(),
		TerminalReason:  row.TerminalReason,
		CreatedAt:       db.TimeFromPg(row.CreatedAt),
		UpdatedAt:       db.TimeFromPg(row.UpdatedAt),
		FinishedAt:      optionalTime(db.TimeFromPg(row.FinishedAt)),
	}
}

func toTaskDependency(row sqlc.OrchestrationTaskDependency) TaskDependency {
	return TaskDependency{
		ID:                       row.ID.String(),
		RunID:                    row.RunID.String(),
		PredecessorTaskID:        row.PredecessorTaskID.String(),
		SuccessorTaskID:          row.SuccessorTaskID.String(),
		PlannerEpoch:             mustUint64FromInt64(row.PlannerEpoch, "task_dependency.planner_epoch"),
		SupersededByPlannerEpoch: mustUint64FromInt64(row.SupersededByPlannerEpoch.Int64, "task_dependency.superseded_by_planner_epoch"),
		CreatedAt:                db.TimeFromPg(row.CreatedAt),
		UpdatedAt:                db.TimeFromPg(row.UpdatedAt),
	}
}

func toTaskResult(row sqlc.OrchestrationTaskResult) TaskResult {
	return TaskResult{
		ID:               row.ID.String(),
		RunID:            row.RunID.String(),
		TaskID:           row.TaskID.String(),
		AttemptID:        row.AttemptID.String(),
		Status:           row.Status,
		Summary:          row.Summary,
		FailureClass:     row.FailureClass,
		RequestReplan:    row.RequestReplan,
		ArtifactIntents:  decodeJSONArrayObjects(row.ArtifactIntents),
		StructuredOutput: decodeJSONObject(row.StructuredOutput),
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		UpdatedAt:        db.TimeFromPg(row.UpdatedAt),
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
		TimeoutAt:                optionalTime(db.TimeFromPg(row.TimeoutAt)),
		ResolvedBy:               row.ResolvedBy,
		ResolvedMode:             row.ResolvedMode,
		ResolvedOptionID:         row.ResolvedOptionID,
		ResolvedFreeformInput:    row.ResolvedFreeformInput,
		ResolvedAt:               optionalTime(db.TimeFromPg(row.ResolvedAt)),
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
		Payload:          sanitizeEventPayload(decodeJSONObject(row.Payload)),
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		PublishedAt:      optionalTime(db.TimeFromPg(row.PublishedAt)),
	}
}

func toRunTimelineEntry(row sqlc.OrchestrationEvent) RunTimelineEntry {
	return RunTimelineEntry{
		Seq:           mustUint64FromInt64(row.Seq, "timeline.seq"),
		Type:          row.Type,
		AggregateType: row.AggregateType,
		AggregateID:   row.AggregateID.String(),
		TaskID:        row.TaskID.String(),
		AttemptID:     row.AttemptID.String(),
		CheckpointID:  row.CheckpointID.String(),
		CreatedAt:     db.TimeFromPg(row.CreatedAt),
		Payload:       sanitizeEventPayload(decodeJSONObject(row.Payload)),
	}
}

func toInspectorAttempt(row sqlc.OrchestrationTaskAttempt) InspectorAttempt {
	return InspectorAttempt{
		ID:               row.ID.String(),
		RunID:            row.RunID.String(),
		TaskID:           row.TaskID.String(),
		AttemptNo:        int(row.AttemptNo),
		WorkerID:         row.WorkerID,
		ExecutorID:       row.ExecutorID,
		Status:           row.Status,
		ClaimEpoch:       mustUint64FromInt64(row.ClaimEpoch, "attempt.claim_epoch"),
		LeaseExpiresAt:   optionalTime(db.TimeFromPg(row.LeaseExpiresAt)),
		LastHeartbeatAt:  optionalTime(db.TimeFromPg(row.LastHeartbeatAt)),
		InputManifestID:  row.InputManifestID.String(),
		ParkCheckpointID: row.ParkCheckpointID.String(),
		FailureClass:     row.FailureClass,
		TerminalReason:   row.TerminalReason,
		StartedAt:        optionalTime(db.TimeFromPg(row.StartedAt)),
		FinishedAt:       optionalTime(db.TimeFromPg(row.FinishedAt)),
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		UpdatedAt:        db.TimeFromPg(row.UpdatedAt),
	}
}

func toInspectorVerification(row sqlc.OrchestrationTaskVerification) InspectorVerification {
	return InspectorVerification{
		ID:              row.ID.String(),
		RunID:           row.RunID.String(),
		TaskID:          row.TaskID.String(),
		ResultID:        row.ResultID.String(),
		AttemptNo:       int(row.AttemptNo),
		WorkerID:        row.WorkerID,
		ExecutorID:      row.ExecutorID,
		VerifierProfile: row.VerifierProfile,
		Status:          row.Status,
		ClaimEpoch:      mustUint64FromInt64(row.ClaimEpoch, "verification.claim_epoch"),
		LeaseExpiresAt:  optionalTime(db.TimeFromPg(row.LeaseExpiresAt)),
		LastHeartbeatAt: optionalTime(db.TimeFromPg(row.LastHeartbeatAt)),
		Verdict:         row.Verdict,
		Summary:         row.Summary,
		FailureClass:    row.FailureClass,
		TerminalReason:  row.TerminalReason,
		StartedAt:       optionalTime(db.TimeFromPg(row.StartedAt)),
		FinishedAt:      optionalTime(db.TimeFromPg(row.FinishedAt)),
		CreatedAt:       db.TimeFromPg(row.CreatedAt),
		UpdatedAt:       db.TimeFromPg(row.UpdatedAt),
	}
}

func toInspectorWorkerLease(row sqlc.OrchestrationWorker) InspectorWorkerLease {
	return InspectorWorkerLease{
		ID:              row.ID,
		ExecutorID:      row.ExecutorID,
		DisplayName:     row.DisplayName,
		Capabilities:    decodeJSONObject(row.Capabilities),
		Status:          row.Status,
		LastHeartbeatAt: db.TimeFromPg(row.LastHeartbeatAt),
		LeaseExpiresAt:  db.TimeFromPg(row.LeaseExpiresAt),
		CreatedAt:       db.TimeFromPg(row.CreatedAt),
		UpdatedAt:       db.TimeFromPg(row.UpdatedAt),
	}
}

func sanitizeEventPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	delete(payload, "claim_token")
	delete(payload, "lease_token")
	return payload
}

func toInputManifest(row sqlc.OrchestrationInputManifest) InputManifest {
	return InputManifest{
		ID:                          row.ID.String(),
		RunID:                       row.RunID.String(),
		TaskID:                      row.TaskID.String(),
		CapturedTaskInputs:          decodeJSONObject(row.CapturedTaskInputs),
		CapturedArtifactVersions:    decodeJSONArrayObjects(row.CapturedArtifactVersions),
		CapturedBlackboardRevisions: decodeJSONArrayObjects(row.CapturedBlackboardRevisions),
		ProjectionHash:              row.ProjectionHash,
		CreatedAt:                   db.TimeFromPg(row.CreatedAt),
	}
}

func toActionRecord(row sqlc.OrchestrationActionLedger) ActionRecord {
	return ActionRecord{
		ID:             row.ID.String(),
		RunID:          row.RunID.String(),
		TaskID:         row.TaskID.String(),
		AttemptID:      uuidToString(row.AttemptID),
		VerificationID: uuidToString(row.VerificationID),
		ActionKind:     strings.TrimSpace(row.ActionKind),
		Status:         strings.TrimSpace(row.Status),
		ToolName:       strings.TrimSpace(row.ToolName),
		ToolCallID:     strings.TrimSpace(row.ToolCallID),
		InputPayload:   decodeJSONValue(row.InputPayload),
		OutputPayload:  decodeJSONValue(row.OutputPayload),
		ErrorPayload:   decodeJSONValue(row.ErrorPayload),
		Summary:        strings.TrimSpace(row.Summary),
		StartedAt:      optionalTime(db.TimeFromPg(row.StartedAt)),
		FinishedAt:     optionalTime(db.TimeFromPg(row.FinishedAt)),
		CreatedAt:      db.TimeFromPg(row.CreatedAt),
		UpdatedAt:      db.TimeFromPg(row.UpdatedAt),
	}
}

func buildRunExecutionSpans(
	attemptRows []sqlc.OrchestrationTaskAttempt,
	verificationRows []sqlc.OrchestrationTaskVerification,
	resultRows []sqlc.OrchestrationTaskResult,
	eventRows []sqlc.OrchestrationEvent,
) []RunExecutionSpan {
	resultByAttemptID := make(map[string]sqlc.OrchestrationTaskResult, len(resultRows))
	resultByID := make(map[string]sqlc.OrchestrationTaskResult, len(resultRows))
	for _, resultRow := range resultRows {
		resultByID[resultRow.ID.String()] = resultRow
		if resultRow.AttemptID.Valid {
			resultByAttemptID[resultRow.AttemptID.String()] = resultRow
		}
	}

	spans := make([]RunExecutionSpan, 0, len(attemptRows)+len(verificationRows))
	attemptSpanByID := make(map[string]*RunExecutionSpan, len(attemptRows))
	for _, attemptRow := range attemptRows {
		span := RunExecutionSpan{
			Kind:               "attempt",
			ID:                 attemptRow.ID.String(),
			RunID:              attemptRow.RunID.String(),
			TaskID:             attemptRow.TaskID.String(),
			AttemptNo:          int(attemptRow.AttemptNo),
			Status:             attemptRow.Status,
			WorkerID:           strings.TrimSpace(attemptRow.WorkerID),
			ExecutorID:         strings.TrimSpace(attemptRow.ExecutorID),
			StartedAt:          optionalTime(db.TimeFromPg(attemptRow.StartedAt)),
			FinishedAt:         optionalTime(db.TimeFromPg(attemptRow.FinishedAt)),
			LastHeartbeatAt:    optionalTime(db.TimeFromPg(attemptRow.LastHeartbeatAt)),
			LeaseExpiresAt:     optionalTime(db.TimeFromPg(attemptRow.LeaseExpiresAt)),
			InputManifestID:    attemptRow.InputManifestID.String(),
			CheckpointID:       attemptRow.ParkCheckpointID.String(),
			FailureClass:       strings.TrimSpace(attemptRow.FailureClass),
			TerminalReason:     strings.TrimSpace(attemptRow.TerminalReason),
			CompletionMetadata: map[string]any{},
			RelatedEventTypes:  make([]string, 0, 6),
		}
		if resultRow, ok := resultByAttemptID[span.ID]; ok {
			span.ResultID = resultRow.ID.String()
			span.Summary = strings.TrimSpace(resultRow.Summary)
			if span.FailureClass == "" {
				span.FailureClass = strings.TrimSpace(resultRow.FailureClass)
			}
		}
		spans = append(spans, span)
		attemptSpanByID[span.ID] = &spans[len(spans)-1]
	}

	verificationSpanByID := make(map[string]*RunExecutionSpan, len(verificationRows))
	for _, verificationRow := range verificationRows {
		span := RunExecutionSpan{
			Kind:               "verification",
			ID:                 verificationRow.ID.String(),
			RunID:              verificationRow.RunID.String(),
			TaskID:             verificationRow.TaskID.String(),
			AttemptNo:          int(verificationRow.AttemptNo),
			Status:             verificationRow.Status,
			WorkerID:           strings.TrimSpace(verificationRow.WorkerID),
			ExecutorID:         strings.TrimSpace(verificationRow.ExecutorID),
			VerifierProfile:    strings.TrimSpace(verificationRow.VerifierProfile),
			StartedAt:          optionalTime(db.TimeFromPg(verificationRow.StartedAt)),
			FinishedAt:         optionalTime(db.TimeFromPg(verificationRow.FinishedAt)),
			LastHeartbeatAt:    optionalTime(db.TimeFromPg(verificationRow.LastHeartbeatAt)),
			LeaseExpiresAt:     optionalTime(db.TimeFromPg(verificationRow.LeaseExpiresAt)),
			ResultID:           verificationRow.ResultID.String(),
			FailureClass:       strings.TrimSpace(verificationRow.FailureClass),
			TerminalReason:     strings.TrimSpace(verificationRow.TerminalReason),
			Summary:            strings.TrimSpace(verificationRow.Summary),
			Verdict:            strings.TrimSpace(verificationRow.Verdict),
			CompletionMetadata: map[string]any{},
			RelatedEventTypes:  make([]string, 0, 6),
		}
		if resultRow, ok := resultByID[span.ResultID]; ok && span.Summary == "" {
			span.Summary = strings.TrimSpace(resultRow.Summary)
		}
		spans = append(spans, span)
		verificationSpanByID[span.ID] = &spans[len(spans)-1]
	}

	for _, eventRow := range eventRows {
		eventType := strings.TrimSpace(eventRow.Type)
		if eventType == "" {
			continue
		}
		switch {
		case strings.HasPrefix(eventType, "run.event.attempt."):
			span := attemptSpanByID[eventRow.AttemptID.String()]
			if span == nil {
				continue
			}
			span.RelatedEventTypes = appendUniqueString(span.RelatedEventTypes, eventType)
			switch eventType {
			case "run.event.attempt.created":
				span.CreatedSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.attempt.created_seq")
			case "run.event.attempt.claimed":
				span.ClaimedSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.attempt.claimed_seq")
			case "run.event.attempt.running":
				span.StartedSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.attempt.started_seq")
			case "run.event.attempt.completed", "run.event.attempt.failed", "run.event.attempt.lost":
				span.TerminalSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.attempt.terminal_seq")
			case "run.event.attempt.requeued":
				span.RequeueSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.attempt.requeue_seq")
			}
		case strings.HasPrefix(eventType, "run.event.verification."):
			payload := decodeJSONObject(eventRow.Payload)
			verificationID := strings.TrimSpace(stringValue(payload["verification_id"]))
			if verificationID == "" {
				verificationID = eventRow.AggregateID.String()
			}
			span := verificationSpanByID[verificationID]
			if span == nil {
				continue
			}
			span.RelatedEventTypes = appendUniqueString(span.RelatedEventTypes, eventType)
			switch eventType {
			case "run.event.verification.created":
				span.CreatedSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.verification.created_seq")
			case "run.event.verification.claimed":
				span.ClaimedSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.verification.claimed_seq")
			case "run.event.verification.running":
				span.StartedSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.verification.started_seq")
			case "run.event.verification.passed", "run.event.verification.rejected", "run.event.verification.lost":
				span.TerminalSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.verification.terminal_seq")
			case "run.event.verification.requeued":
				span.RequeueSeq = mustUint64FromInt64(eventRow.Seq, "execution_span.verification.requeue_seq")
			}
		}
	}

	sort.SliceStable(spans, func(i, j int) bool {
		leftSeq := executionSpanSortSeq(spans[i])
		rightSeq := executionSpanSortSeq(spans[j])
		if leftSeq != rightSeq {
			return leftSeq < rightSeq
		}
		if spans[i].TaskID != spans[j].TaskID {
			return spans[i].TaskID < spans[j].TaskID
		}
		if spans[i].Kind != spans[j].Kind {
			return spans[i].Kind < spans[j].Kind
		}
		return spans[i].ID < spans[j].ID
	})

	return spans
}

func executionSpanSortSeq(span RunExecutionSpan) uint64 {
	if span.CreatedSeq != 0 {
		return span.CreatedSeq
	}
	if span.ClaimedSeq != 0 {
		return span.ClaimedSeq
	}
	if span.StartedSeq != 0 {
		return span.StartedSeq
	}
	if span.TerminalSeq != 0 {
		return span.TerminalSeq
	}
	if span.RequeueSeq != 0 {
		return span.RequeueSeq
	}
	return 0
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func summarizeRunInspector(tasks []Task, checkpoints []HumanCheckpoint, attempts []InspectorAttempt, verifications []InspectorVerification, workers []InspectorWorkerLease, stuckSignals []RunStuckSignal) RunInspectorSummary {
	summary := RunInspectorSummary{}
	for _, checkpoint := range checkpoints {
		if checkpoint.Status == CheckpointStatusOpen {
			summary.OpenCheckpointCount++
		}
	}
	for _, task := range tasks {
		switch task.Status {
		case TaskStatusReady:
			summary.ReadyTaskCount++
		case TaskStatusDispatching:
			summary.DispatchingTaskCount++
		case TaskStatusRunning:
			summary.RunningTaskCount++
		case TaskStatusVerifying:
			summary.VerifyingTaskCount++
		case TaskStatusWaitingHuman:
			summary.WaitingHumanTaskCount++
		case TaskStatusCompleted:
			summary.CompletedTaskCount++
		case TaskStatusBlocked:
			summary.BlockedTaskCount++
		case TaskStatusFailed:
			summary.FailedTaskCount++
		case TaskStatusCancelled:
			summary.CancelledTaskCount++
		}
	}
	for _, attempt := range attempts {
		switch attempt.Status {
		case TaskAttemptStatusCreated, TaskAttemptStatusClaimed, TaskAttemptStatusBinding, TaskAttemptStatusRunning:
			summary.ActiveAttemptCount++
		}
	}
	for _, verification := range verifications {
		switch verification.Status {
		case TaskVerificationStatusCreated, TaskVerificationStatusClaimed, TaskVerificationStatusRunning:
			summary.ActiveVerificationCount++
		}
	}
	for _, worker := range workers {
		if worker.Status == WorkerStatusActive {
			summary.ActiveWorkerCount++
		}
	}
	summary.StuckSignalCount = len(stuckSignals)
	for _, signal := range stuckSignals {
		if signal.Severity == "critical" {
			summary.CriticalSignalCount++
		}
	}
	return summary
}

func buildRunStuckSignals(tasks []Task, attempts []InspectorAttempt, verifications []InspectorVerification, workers []InspectorWorkerLease, observedAt time.Time) []RunStuckSignal {
	activeAttemptsByTask := make(map[string][]InspectorAttempt)
	activeVerificationsByTask := make(map[string][]InspectorVerification)
	activeWorkByWorker := make(map[string]struct{})
	signals := make([]RunStuckSignal, 0)

	for _, attempt := range attempts {
		if !isActiveAttemptStatus(attempt.Status) {
			continue
		}
		activeAttemptsByTask[attempt.TaskID] = append(activeAttemptsByTask[attempt.TaskID], attempt)
		if workerID := strings.TrimSpace(attempt.WorkerID); workerID != "" {
			activeWorkByWorker[workerID] = struct{}{}
		}
		if inspectorLeaseExpired(attempt.LeaseExpiresAt, observedAt) {
			signals = append(signals, RunStuckSignal{
				Code:            "attempt_lease_expired",
				Severity:        "critical",
				Message:         "active attempt lease expired before recovery completed",
				TaskID:          attempt.TaskID,
				AttemptID:       attempt.ID,
				WorkerID:        attempt.WorkerID,
				Status:          attempt.Status,
				LastHeartbeatAt: attempt.LastHeartbeatAt,
				LeaseExpiresAt:  attempt.LeaseExpiresAt,
				ObservedAt:      observedAt,
			})
		}
	}
	for _, verification := range verifications {
		if !isActiveVerificationStatus(verification.Status) {
			continue
		}
		activeVerificationsByTask[verification.TaskID] = append(activeVerificationsByTask[verification.TaskID], verification)
		if workerID := strings.TrimSpace(verification.WorkerID); workerID != "" {
			activeWorkByWorker[workerID] = struct{}{}
		}
		if inspectorLeaseExpired(verification.LeaseExpiresAt, observedAt) {
			signals = append(signals, RunStuckSignal{
				Code:            "verification_lease_expired",
				Severity:        "critical",
				Message:         "active verification lease expired before recovery completed",
				TaskID:          verification.TaskID,
				VerificationID:  verification.ID,
				WorkerID:        verification.WorkerID,
				Status:          verification.Status,
				LastHeartbeatAt: verification.LastHeartbeatAt,
				LeaseExpiresAt:  verification.LeaseExpiresAt,
				ObservedAt:      observedAt,
			})
		}
	}
	for _, task := range tasks {
		switch task.Status {
		case TaskStatusDispatching, TaskStatusRunning:
			if len(activeAttemptsByTask[task.ID]) == 0 {
				signals = append(signals, RunStuckSignal{
					Code:       "task_missing_active_attempt",
					Severity:   "critical",
					Message:    "task is active but has no active attempt",
					TaskID:     task.ID,
					Status:     task.Status,
					ObservedAt: observedAt,
				})
			}
		case TaskStatusVerifying:
			if len(activeVerificationsByTask[task.ID]) == 0 {
				signals = append(signals, RunStuckSignal{
					Code:       "task_missing_active_verification",
					Severity:   "critical",
					Message:    "task is verifying but has no active verification",
					TaskID:     task.ID,
					Status:     task.Status,
					ObservedAt: observedAt,
				})
			}
		}
	}
	for _, worker := range workers {
		if worker.Status != WorkerStatusActive {
			continue
		}
		if _, ok := activeWorkByWorker[worker.ID]; !ok {
			continue
		}
		if !inspectorLeaseExpired(optionalTime(worker.LeaseExpiresAt), observedAt) {
			continue
		}
		signals = append(signals, RunStuckSignal{
			Code:            "worker_lease_expired",
			Severity:        "critical",
			Message:         "worker lease expired while run still references it as active",
			WorkerID:        worker.ID,
			Status:          worker.Status,
			LastHeartbeatAt: optionalTime(worker.LastHeartbeatAt),
			LeaseExpiresAt:  optionalTime(worker.LeaseExpiresAt),
			ObservedAt:      observedAt,
		})
	}

	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Severity != signals[j].Severity {
			return signals[i].Severity < signals[j].Severity
		}
		if signals[i].Code != signals[j].Code {
			return signals[i].Code < signals[j].Code
		}
		if signals[i].TaskID != signals[j].TaskID {
			return signals[i].TaskID < signals[j].TaskID
		}
		if signals[i].AttemptID != signals[j].AttemptID {
			return signals[i].AttemptID < signals[j].AttemptID
		}
		if signals[i].VerificationID != signals[j].VerificationID {
			return signals[i].VerificationID < signals[j].VerificationID
		}
		return signals[i].WorkerID < signals[j].WorkerID
	})

	return signals
}

func isActiveAttemptStatus(status string) bool {
	switch status {
	case TaskAttemptStatusCreated, TaskAttemptStatusClaimed, TaskAttemptStatusBinding, TaskAttemptStatusRunning:
		return true
	default:
		return false
	}
}

func isActiveVerificationStatus(status string) bool {
	switch status {
	case TaskVerificationStatusCreated, TaskVerificationStatusClaimed, TaskVerificationStatusRunning:
		return true
	default:
		return false
	}
}

func inspectorLeaseExpired(leaseExpiresAt *time.Time, observedAt time.Time) bool {
	return leaseExpiresAt != nil && !leaseExpiresAt.After(observedAt)
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	cloned := value
	return &cloned
}
