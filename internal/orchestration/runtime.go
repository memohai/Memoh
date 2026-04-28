package orchestration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type AttemptAssignmentPublisher interface {
	PublishAttemptAssignment(ctx context.Context, attempt TaskAttempt) error
}

type noopAttemptAssignmentPublisher struct{}

func (noopAttemptAssignmentPublisher) PublishAttemptAssignment(context.Context, TaskAttempt) error {
	return nil
}

func (s *Service) SetAttemptAssignmentPublisher(publisher AttemptAssignmentPublisher) {
	if publisher == nil {
		publisher = noopAttemptAssignmentPublisher{}
	}
	s.attemptAssignments = publisher
}

func (s *Service) RegisterWorker(ctx context.Context, req WorkerRegistration) (*WorkerLease, error) {
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, fmt.Errorf("%w: worker_id is required", ErrInvalidArgument)
	}
	executorID := strings.TrimSpace(req.ExecutorID)
	if executorID == "" {
		executorID = DefaultWorkerExecutorID
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = DefaultWorkerDisplayName
	}
	leaseToken := strings.TrimSpace(req.LeaseToken)
	if leaseToken == "" {
		leaseToken = uuid.NewString()
	}
	row, err := s.queries.UpsertOrchestrationWorker(ctx, sqlc.UpsertOrchestrationWorkerParams{
		ID:             workerID,
		ExecutorID:     executorID,
		DisplayName:    displayName,
		Capabilities:   marshalObject(req.Capabilities),
		Status:         WorkerStatusActive,
		LeaseToken:     leaseToken,
		LeaseExpiresAt: timeToPg(time.Now().Add(normalizeLeaseTTL(req.LeaseTTLSeconds, TaskAttemptDefaultLeaseTTL))),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkerLeaseConflict
		}
		return nil, fmt.Errorf("register orchestration worker: %w", err)
	}
	lease := toWorkerLease(row)
	return &lease, nil
}

func (s *Service) HeartbeatWorker(ctx context.Context, workerID, leaseToken string, leaseTTLSeconds int) (*WorkerLease, error) {
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return nil, fmt.Errorf("%w: worker id is required", ErrInvalidArgument)
	}
	trimmedLeaseToken := strings.TrimSpace(leaseToken)
	if trimmedLeaseToken == "" {
		return nil, fmt.Errorf("%w: worker lease token is required", ErrInvalidArgument)
	}
	row, err := s.queries.HeartbeatOrchestrationWorker(ctx, sqlc.HeartbeatOrchestrationWorkerParams{
		ID:             trimmedWorkerID,
		Status:         WorkerStatusActive,
		LeaseToken:     trimmedLeaseToken,
		LeaseExpiresAt: timeToPg(time.Now().Add(normalizeLeaseTTL(leaseTTLSeconds, TaskAttemptDefaultLeaseTTL))),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkerLeaseConflict
		}
		return nil, fmt.Errorf("heartbeat orchestration worker: %w", err)
	}
	lease := toWorkerLease(row)
	return &lease, nil
}

func (s *Service) ProcessNextPlanningIntent(ctx context.Context) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin planning intent tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	intent, err := qtx.ClaimNextOrchestrationPlanningIntent(ctx, sqlc.ClaimNextOrchestrationPlanningIntentParams{
		ClaimToken:     uuid.NewString(),
		ClaimedBy:      "planner",
		LeaseExpiresAt: timeToPg(time.Now().Add(PlanningIntentDefaultLeaseTTL)),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("claim planning intent: %w", err)
	}

	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, intent.RunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			failedIntent, failErr := qtx.FailOrchestrationPlanningIntent(ctx, sqlc.FailOrchestrationPlanningIntentParams{
				ID:            intent.ID,
				ClaimToken:    intent.ClaimToken,
				FailureReason: ErrRunNotFound.Error(),
			})
			if failErr != nil {
				return false, fmt.Errorf("fail orphaned planning intent: %w", failErr)
			}
			if _, err := s.appendEvent(ctx, qtx, intent.RunID, eventSpec{
				TaskID:           intent.TaskID,
				CheckpointID:     intent.CheckpointID,
				AggregateType:    "planning_intent",
				AggregateID:      failedIntent.ID,
				AggregateVersion: failedIntent.ClaimEpoch,
				Type:             "run.event.planning_intent.failed",
				Payload: map[string]any{
					"planning_intent_id": failedIntent.ID.String(),
					"kind":               failedIntent.Kind,
					"status":             failedIntent.Status,
					"failure_reason":     failedIntent.FailureReason,
				},
			}); err != nil && !errors.Is(err, ErrRunNotFound) {
				return false, err
			}
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit failed missing-run planning intent tx: %w", err)
			}
			return true, nil
		}
		return false, fmt.Errorf("lock run for planning intent: %w", err)
	}
	if !runAcceptsPlanningIntent(intent.Kind, runRow.LifecycleStatus, intent.BasePlannerEpoch, runRow.PlannerEpoch) {
		reason := fmt.Sprintf("stale planning intent for run status=%s base_epoch=%d current_epoch=%d", runRow.LifecycleStatus, intent.BasePlannerEpoch, runRow.PlannerEpoch)
		_, failErr := qtx.FailOrchestrationPlanningIntent(ctx, sqlc.FailOrchestrationPlanningIntentParams{
			ID:            intent.ID,
			ClaimToken:    intent.ClaimToken,
			FailureReason: reason,
		})
		if failErr != nil {
			return false, fmt.Errorf("fail stale planning intent: %w", failErr)
		}
		if err := s.syncRunPlanningStatus(ctx, qtx, runRow.ID); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit stale planning intent tx: %w", err)
		}
		return true, nil
	}

	var lastEvent sqlc.OrchestrationEvent
	switch intent.Kind {
	case PlanningIntentKindStartRun:
		lastEvent, err = s.processStartRunPlanningIntent(ctx, qtx, runRow, intent)
	case PlanningIntentKindCheckpointResume:
		lastEvent, err = s.processCheckpointResumePlanningIntent(ctx, qtx, runRow, intent)
	case PlanningIntentKindAttemptFinalize:
		lastEvent, err = s.processAttemptFinalizePlanningIntent(ctx, qtx, runRow, intent)
	case PlanningIntentKindReplan:
		lastEvent, err = s.processReplanPlanningIntent(ctx, qtx, runRow, intent)
	default:
		_, failErr := qtx.FailOrchestrationPlanningIntent(ctx, sqlc.FailOrchestrationPlanningIntentParams{
			ID:            intent.ID,
			ClaimToken:    intent.ClaimToken,
			FailureReason: fmt.Sprintf("unsupported planning intent kind %q", intent.Kind),
		})
		if failErr != nil {
			return false, fmt.Errorf("fail unsupported planning intent: %w", failErr)
		}
		if err := s.syncRunPlanningStatus(ctx, qtx, runRow.ID); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit failed unsupported planning intent tx: %w", err)
		}
		return true, nil
	}
	if err != nil {
		if shouldFailPlanningIntent(err) {
			failedIntent, failErr := qtx.FailOrchestrationPlanningIntent(ctx, sqlc.FailOrchestrationPlanningIntentParams{
				ID:            intent.ID,
				ClaimToken:    intent.ClaimToken,
				FailureReason: err.Error(),
			})
			if failErr != nil {
				return false, fmt.Errorf("fail planning intent after handler error: %w", failErr)
			}
			if err := s.syncRunPlanningStatus(ctx, qtx, runRow.ID); err != nil {
				return false, err
			}
			runRow, err = qtx.GetOrchestrationRunByIDForUpdate(ctx, runRow.ID)
			if err != nil {
				return false, fmt.Errorf("lock run after planning intent failure: %w", err)
			}
			if completionEvent, completed, err := s.maybeMarkRunCompletedAfterPlanningIntent(ctx, qtx, runRow, failedIntent); err != nil {
				return false, err
			} else if completed {
				lastEvent = completionEvent
			}
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit failed planning intent tx: %w", err)
			}
			return true, nil
		}
		return false, err
	}

	completedIntent, err := qtx.CompleteOrchestrationPlanningIntent(ctx, sqlc.CompleteOrchestrationPlanningIntentParams{
		ID:         intent.ID,
		ClaimToken: intent.ClaimToken,
	})
	if err != nil {
		return false, fmt.Errorf("complete planning intent: %w", err)
	}
	if err := s.syncRunPlanningStatus(ctx, qtx, runRow.ID); err != nil {
		return false, err
	}
	lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		TaskID:           intent.TaskID,
		CheckpointID:     intent.CheckpointID,
		AggregateType:    "planning_intent",
		AggregateID:      completedIntent.ID,
		AggregateVersion: completedIntent.ClaimEpoch,
		Type:             "run.event.planning_intent.completed",
		Payload: map[string]any{
			"planning_intent_id": completedIntent.ID.String(),
			"kind":               completedIntent.Kind,
			"status":             completedIntent.Status,
			"last_event_seq":     lastEvent.Seq,
		},
	})
	if err != nil {
		return false, err
	}
	runRow, err = qtx.GetOrchestrationRunByIDForUpdate(ctx, runRow.ID)
	if err != nil {
		return false, fmt.Errorf("lock run after planning intent completion: %w", err)
	}
	if completionEvent, completed, err := s.maybeMarkRunCompletedAfterPlanningIntent(ctx, qtx, runRow, completedIntent); err != nil {
		return false, err
	} else if completed {
		lastEvent = completionEvent
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit planning intent tx: %w", err)
	}
	_ = lastEvent
	return true, nil
}

func (s *Service) processStartRunPlanningIntent(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, intent sqlc.OrchestrationPlanningIntent) (sqlc.OrchestrationEvent, error) {
	if !intent.TaskID.Valid {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: start_run intent missing task", ErrPlanningIntentNotFound)
	}
	taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, intent.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationEvent{}, ErrTaskNotFound
		}
		return sqlc.OrchestrationEvent{}, fmt.Errorf("lock task for start_run intent: %w", err)
	}
	var lastEvent sqlc.OrchestrationEvent
	if taskRow.Status == TaskStatusCreated {
		readyTask, err := qtx.MarkOrchestrationTaskReadyFromCheckpoint(ctx, taskRow.ID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("mark task ready from start_run intent: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           readyTask.ID,
			AggregateType:    "task",
			AggregateID:      readyTask.ID,
			AggregateVersion: readyTask.StatusVersion,
			Type:             "run.event.task.ready",
			Payload: map[string]any{
				"task_id":         readyTask.ID.String(),
				"previous_status": taskRow.Status,
				"new_status":      readyTask.Status,
				"ready_reason":    "planning_intent.start_run",
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}
	if runRow.LifecycleStatus == LifecycleStatusCreated {
		runningRun, err := qtx.MarkOrchestrationRunRunning(ctx, runRow.ID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("mark run running from start_run intent: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			AggregateType:    "run",
			AggregateID:      runningRun.ID,
			AggregateVersion: runningRun.StatusVersion,
			Type:             "run.event.running",
			Payload: map[string]any{
				"run_id":          runningRun.ID.String(),
				"previous_status": runRow.LifecycleStatus,
				"new_status":      runningRun.LifecycleStatus,
				"entry_reason":    "planning_intent.start_run",
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}
	return lastEvent, nil
}

func (s *Service) processCheckpointResumePlanningIntent(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, intent sqlc.OrchestrationPlanningIntent) (sqlc.OrchestrationEvent, error) {
	if !intent.TaskID.Valid {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: checkpoint_resume intent missing task", ErrPlanningIntentNotFound)
	}
	if !intent.CheckpointID.Valid {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: checkpoint_resume intent missing checkpoint", ErrPlanningIntentNotFound)
	}

	checkpointRow, err := qtx.GetOrchestrationHumanCheckpointByIDForUpdate(ctx, intent.CheckpointID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationEvent{}, ErrCheckpointNotFound
		}
		return sqlc.OrchestrationEvent{}, fmt.Errorf("lock checkpoint for checkpoint_resume intent: %w", err)
	}
	if checkpointRow.RunID != runRow.ID {
		return sqlc.OrchestrationEvent{}, ErrCheckpointNotFound
	}
	if checkpointRow.Status != CheckpointStatusResolved {
		return sqlc.OrchestrationEvent{}, ErrCheckpointNotOpen
	}

	taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, intent.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationEvent{}, ErrTaskNotFound
		}
		return sqlc.OrchestrationEvent{}, fmt.Errorf("lock task for checkpoint_resume intent: %w", err)
	}
	if taskRow.RunID != runRow.ID {
		return sqlc.OrchestrationEvent{}, ErrTaskNotFound
	}

	var lastEvent sqlc.OrchestrationEvent
	if taskRow.Status == TaskStatusWaitingHuman && taskRow.WaitingCheckpointID.Valid && taskRow.WaitingCheckpointID == checkpointRow.ID {
		openBarrier, hasOpenBarrier, err := findOpenRunBlockingCheckpoint(ctx, qtx, runRow.ID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("find open run blocking checkpoint: %w", err)
		}
		if hasOpenBarrier {
			waitingTask, err := qtx.MarkOrchestrationTaskWaitingHuman(ctx, sqlc.MarkOrchestrationTaskWaitingHumanParams{
				ID:                  taskRow.ID,
				WaitingCheckpointID: openBarrier.ID,
				WaitingScope:        "run",
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("rebind task to open run checkpoint: %w", err)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
				TaskID:           waitingTask.ID,
				CheckpointID:     openBarrier.ID,
				AggregateType:    "task",
				AggregateID:      waitingTask.ID,
				AggregateVersion: waitingTask.StatusVersion,
				Type:             "run.event.task.waiting_human",
				Payload: map[string]any{
					"task_id":                        waitingTask.ID.String(),
					"previous_status":                taskRow.Status,
					"new_status":                     waitingTask.Status,
					"waiting_scope":                  waitingTask.WaitingScope,
					"previous_waiting_scope":         taskRow.WaitingScope,
					"previous_waiting_checkpoint_id": checkpointRow.ID.String(),
					"waiting_checkpoint_id":          openBarrier.ID.String(),
				},
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
		} else {
			readyTask, err := qtx.MarkOrchestrationTaskReadyFromCheckpoint(ctx, taskRow.ID)
			if err != nil {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("mark checkpoint task ready: %w", err)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
				TaskID:           readyTask.ID,
				CheckpointID:     checkpointRow.ID,
				AggregateType:    "task",
				AggregateID:      readyTask.ID,
				AggregateVersion: readyTask.StatusVersion,
				Type:             "run.event.task.ready",
				Payload: map[string]any{
					"task_id":                        readyTask.ID.String(),
					"previous_status":                taskRow.Status,
					"new_status":                     readyTask.Status,
					"ready_reason":                   "planning_intent.checkpoint_resolved",
					"previous_waiting_scope":         taskRow.WaitingScope,
					"previous_waiting_checkpoint_id": checkpointRow.ID.String(),
					"checkpoint_id":                  checkpointRow.ID.String(),
				},
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
		}
	}

	if checkpointRow.BlocksRun && runRow.LifecycleStatus == LifecycleStatusWaitingHuman {
		lastEvent, err = s.releaseRunBarrierAfterCheckpointClosure(ctx, qtx, runRow, checkpointRow, pgtype.UUID{}, "planning_intent.checkpoint_resolved")
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}

	if !lastEvent.ID.Valid {
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           intent.TaskID,
			CheckpointID:     intent.CheckpointID,
			AggregateType:    "planning_intent",
			AggregateID:      intent.ID,
			AggregateVersion: intent.ClaimEpoch,
			Type:             "run.event.planning_intent.checkpoint_resume.noop",
			Payload: map[string]any{
				"planning_intent_id": intent.ID.String(),
				"task_id":            intent.TaskID.String(),
				"checkpoint_id":      intent.CheckpointID.String(),
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}

	return lastEvent, nil
}

func (s *Service) processAttemptFinalizePlanningIntent(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, intent sqlc.OrchestrationPlanningIntent) (sqlc.OrchestrationEvent, error) {
	if !intent.TaskID.Valid {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: attempt_finalize intent missing task", ErrPlanningIntentNotFound)
	}
	payload := decodeJSONObject(intent.Payload)
	attemptID, _ := payload["attempt_id"].(string)
	if strings.TrimSpace(attemptID) == "" {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: attempt_finalize intent missing attempt", ErrPlanningIntentNotFound)
	}
	attemptUUID, err := db.ParseUUID(attemptID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: invalid attempt_finalize attempt id", ErrPlanningIntentNotFound)
	}

	attemptRow, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, attemptUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationEvent{}, ErrAttemptNotFound
		}
		return sqlc.OrchestrationEvent{}, fmt.Errorf("lock attempt for attempt_finalize intent: %w", err)
	}
	if attemptRow.RunID != runRow.ID || attemptRow.TaskID != intent.TaskID {
		return sqlc.OrchestrationEvent{}, ErrAttemptNotFound
	}

	taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, intent.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationEvent{}, ErrTaskNotFound
		}
		return sqlc.OrchestrationEvent{}, fmt.Errorf("lock task for attempt_finalize intent: %w", err)
	}
	if taskRow.RunID != runRow.ID {
		return sqlc.OrchestrationEvent{}, ErrTaskNotFound
	}

	resultID, _ := payload["result_id"].(string)
	completionStatus, _ := payload["completion_status"].(string)
	terminalReason, _ := payload["terminal_reason"].(string)
	failureClass, _ := payload["failure_class"].(string)
	structuredOutput, _ := payload["structured_output"].(map[string]any)
	requestReplan, _ := payload["request_replan"].(bool)
	var resultUUID pgtype.UUID
	if strings.TrimSpace(resultID) != "" {
		resultUUID, err = db.ParseUUID(resultID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: invalid attempt_finalize result id", ErrPlanningIntentNotFound)
		}
	}

	var lastEvent sqlc.OrchestrationEvent
	switch completionStatus {
	case TaskAttemptStatusCompleted:
		if taskRow.Status == TaskStatusRunning {
			verificationPolicy := decodeJSONObject(taskRow.VerificationPolicy)
			if requiresTaskVerification(verificationPolicy) {
				verifyingTask, err := qtx.MarkOrchestrationTaskVerifying(ctx, sqlc.MarkOrchestrationTaskVerifyingParams{
					ID:             taskRow.ID,
					LatestResultID: resultUUID,
				})
				if err != nil {
					return sqlc.OrchestrationEvent{}, fmt.Errorf("mark task verifying from attempt_finalize: %w", err)
				}
				lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
					TaskID:           verifyingTask.ID,
					AttemptID:        attemptRow.ID,
					AggregateType:    "task",
					AggregateID:      verifyingTask.ID,
					AggregateVersion: verifyingTask.StatusVersion,
					Type:             "run.event.task.verifying",
					Payload: map[string]any{
						"task_id":             verifyingTask.ID.String(),
						"attempt_id":          attemptRow.ID.String(),
						"previous_status":     taskRow.Status,
						"new_status":          verifyingTask.Status,
						"latest_result_id":    resultID,
						"verification_policy": verificationPolicy,
					},
				})
				if err != nil {
					return sqlc.OrchestrationEvent{}, err
				}
				_, verificationUUID, err := newPGUUID()
				if err != nil {
					return sqlc.OrchestrationEvent{}, err
				}
				verificationRow, err := qtx.CreateOrchestrationTaskVerification(ctx, sqlc.CreateOrchestrationTaskVerificationParams{
					ID:              verificationUUID,
					RunID:           runRow.ID,
					TaskID:          taskRow.ID,
					ResultID:        resultUUID,
					AttemptNo:       attemptRow.AttemptNo,
					VerifierProfile: verifierProfileForTaskPolicy(verificationPolicy),
					Status:          TaskVerificationStatusCreated,
				})
				if err != nil {
					return sqlc.OrchestrationEvent{}, fmt.Errorf("create task verification from attempt_finalize: %w", err)
				}
				lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
					TaskID:           verificationRow.TaskID,
					AttemptID:        attemptRow.ID,
					AggregateType:    "verification",
					AggregateID:      verificationRow.ID,
					AggregateVersion: 1,
					Type:             "run.event.verification.created",
					Payload: map[string]any{
						"verification_id":  verificationRow.ID.String(),
						"task_id":          verificationRow.TaskID.String(),
						"result_id":        verificationRow.ResultID.String(),
						"attempt_id":       attemptRow.ID.String(),
						"status":           verificationRow.Status,
						"verifier_profile": verificationRow.VerifierProfile,
					},
				})
				if err != nil {
					return sqlc.OrchestrationEvent{}, err
				}
				break
			}

			completedTask, err := qtx.MarkOrchestrationTaskCompleted(ctx, sqlc.MarkOrchestrationTaskCompletedParams{
				ID:             taskRow.ID,
				LatestResultID: resultUUID,
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("mark task completed from attempt_finalize: %w", err)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
				TaskID:           completedTask.ID,
				AttemptID:        attemptRow.ID,
				AggregateType:    "task",
				AggregateID:      completedTask.ID,
				AggregateVersion: completedTask.StatusVersion,
				Type:             "run.event.task.completed",
				Payload: map[string]any{
					"task_id":           completedTask.ID.String(),
					"attempt_id":        attemptRow.ID.String(),
					"previous_status":   taskRow.Status,
					"new_status":        completedTask.Status,
					"latest_result_id":  resultID,
					"structured_output": structuredOutput,
				},
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
			if requestReplan {
				childPlans := decodePlannedChildTasks(structuredOutput)
				if len(childPlans) == 0 {
					return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: request_replan requires structured_output.child_tasks", ErrPlanningIntentInvalid)
				}
				if err := validatePlannedChildTasks(childPlans); err != nil {
					return sqlc.OrchestrationEvent{}, err
				}
				lastEvent, err = s.enqueueReplanPlanningIntent(ctx, qtx, runRow, completedTask, attemptRow, childPlans, "attempt_finalize.request_replan")
				if err != nil {
					return sqlc.OrchestrationEvent{}, err
				}
			} else if completionEvent, completed, err := s.maybeMarkRunCompletedAfterTaskTransition(ctx, qtx, runRow, completedTask.ID, attemptRow.ID); err != nil {
				return sqlc.OrchestrationEvent{}, err
			} else if completed {
				lastEvent = completionEvent
			}
		}
	case TaskAttemptStatusFailed, TaskAttemptStatusLost:
		if taskRow.Status == TaskStatusRunning {
			failedTask, err := qtx.MarkOrchestrationTaskFailed(ctx, sqlc.MarkOrchestrationTaskFailedParams{
				ID:             taskRow.ID,
				LatestResultID: resultUUID,
				TerminalReason: normalizeAttemptTerminalReason(terminalReason),
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("mark task failed from attempt_finalize: %w", err)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
				TaskID:           failedTask.ID,
				AttemptID:        attemptRow.ID,
				AggregateType:    "task",
				AggregateID:      failedTask.ID,
				AggregateVersion: failedTask.StatusVersion,
				Type:             "run.event.task.failed",
				Payload: map[string]any{
					"task_id":          failedTask.ID.String(),
					"attempt_id":       attemptRow.ID.String(),
					"previous_status":  taskRow.Status,
					"new_status":       failedTask.Status,
					"latest_result_id": resultID,
					"failure_class":    failureClass,
					"terminal_reason":  failedTask.TerminalReason,
				},
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
			if err := s.propagateTaskFailureToDependents(ctx, qtx, failedTask, attemptRow); err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
			if err := s.markRunFailedFromTaskFailure(ctx, qtx, runRow, failedTask, attemptRow); err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
		}
	default:
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: unsupported attempt_finalize status %q", ErrPlanningIntentNotFound, completionStatus)
	}

	if !lastEvent.ID.Valid {
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           intent.TaskID,
			AttemptID:        attemptUUID,
			AggregateType:    "planning_intent",
			AggregateID:      intent.ID,
			AggregateVersion: intent.ClaimEpoch,
			Type:             "run.event.planning_intent.attempt_finalize.noop",
			Payload: map[string]any{
				"planning_intent_id": intent.ID.String(),
				"task_id":            intent.TaskID.String(),
				"attempt_id":         attemptID,
				"completion_status":  completionStatus,
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}

	return lastEvent, nil
}

func (s *Service) enqueueReplanPlanningIntent(
	ctx context.Context,
	qtx *sqlc.Queries,
	runRow sqlc.OrchestrationRun,
	sourceTask sqlc.OrchestrationTask,
	attemptRow sqlc.OrchestrationTaskAttempt,
	childPlans []plannedChildTask,
	reason string,
) (sqlc.OrchestrationEvent, error) {
	_, planningIntentUUID, err := newPGUUID()
	if err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	replacementPlan := map[string]any{
		"child_tasks": encodePlannedChildTasks(childPlans),
	}
	planningIntent, err := qtx.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               planningIntentUUID,
		RunID:            runRow.ID,
		TaskID:           sourceTask.ID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindReplan,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: runRow.PlannerEpoch,
		Payload: marshalJSON(map[string]any{
			"run_id":             runRow.ID.String(),
			"source_task_id":     sourceTask.ID.String(),
			"attempt_id":         uuidString(attemptRow.ID),
			"reason":             strings.TrimSpace(reason),
			"base_planner_epoch": runRow.PlannerEpoch,
			"replacement_plan":   replacementPlan,
		}),
	})
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("create replan planning intent: %w", err)
	}
	event, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		TaskID:           sourceTask.ID,
		AttemptID:        attemptRow.ID,
		AggregateType:    "planning_intent",
		AggregateID:      planningIntent.ID,
		AggregateVersion: 1,
		Type:             "run.event.planning_intent.enqueued",
		Payload: map[string]any{
			"planning_intent_id": planningIntent.ID.String(),
			"run_id":             runRow.ID.String(),
			"task_id":            sourceTask.ID.String(),
			"attempt_id":         uuidString(attemptRow.ID),
			"kind":               planningIntent.Kind,
			"status":             planningIntent.Status,
			"reason":             strings.TrimSpace(reason),
		},
	})
	if err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	return event, nil
}

func (s *Service) processReplanPlanningIntent(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, intent sqlc.OrchestrationPlanningIntent) (sqlc.OrchestrationEvent, error) {
	if !intent.TaskID.Valid {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: replan intent missing task", ErrPlanningIntentNotFound)
	}
	payload := decodeJSONObject(intent.Payload)
	sourceTaskID, _ := payload["source_task_id"].(string)
	if strings.TrimSpace(sourceTaskID) == "" {
		sourceTaskID = intent.TaskID.String()
	}
	sourceTaskUUID, err := db.ParseUUID(sourceTaskID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: invalid replan source task id", ErrPlanningIntentNotFound)
	}
	sourceTask, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, sourceTaskUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationEvent{}, ErrTaskNotFound
		}
		return sqlc.OrchestrationEvent{}, fmt.Errorf("lock replan source task: %w", err)
	}
	if sourceTask.RunID != runRow.ID {
		return sqlc.OrchestrationEvent{}, ErrTaskNotFound
	}
	if sourceTask.SupersededByPlannerEpoch.Valid {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: source task already superseded", ErrPlanningIntentInvalid)
	}

	childPlans := decodeReplanChildTasks(payload)
	if len(childPlans) == 0 {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: replan intent requires replacement_plan.child_tasks", ErrPlanningIntentInvalid)
	}
	if err := validatePlannedChildTasks(childPlans); err != nil {
		return sqlc.OrchestrationEvent{}, err
	}

	attemptID := pgtype.UUID{}
	if rawAttemptID, _ := payload["attempt_id"].(string); strings.TrimSpace(rawAttemptID) != "" {
		attemptID, err = db.ParseUUID(rawAttemptID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: invalid replan attempt id", ErrPlanningIntentNotFound)
		}
	}

	allTasks, err := qtx.ListCurrentOrchestrationTasksByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("list current tasks for replan: %w", err)
	}
	subtreeTaskIDs := buildTaskSubtreeSet(allTasks, sourceTask.ID)
	allDependencies, err := qtx.ListCurrentOrchestrationTaskDependenciesByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("list current task dependencies for replan: %w", err)
	}
	if err := validateReplanSubtreeIsQuiescent(ctx, qtx, runRow.ID, subtreeTaskIDs, allTasks, allDependencies); err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	allCheckpoints, err := qtx.ListCurrentOrchestrationCheckpointsByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("list current checkpoints for replan: %w", err)
	}

	advancedRun, err := qtx.AdvanceOrchestrationRunPlannerEpoch(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("advance run planner epoch: %w", err)
	}
	newPlannerEpoch := advancedRun.PlannerEpoch
	newPlannerEpochValue := pgtype.Int8{Int64: newPlannerEpoch, Valid: true}
	lastEvent, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		TaskID:           sourceTask.ID,
		AggregateType:    "run",
		AggregateID:      advancedRun.ID,
		AggregateVersion: newPlannerEpoch,
		Type:             "run.event.planner_epoch.advanced",
		Payload: map[string]any{
			"run_id":                 advancedRun.ID.String(),
			"source_task_id":         sourceTask.ID.String(),
			"planning_intent_id":     intent.ID.String(),
			"previous_planner_epoch": runRow.PlannerEpoch,
			"new_planner_epoch":      newPlannerEpoch,
		},
	})
	if err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	expandEvent, err := s.expandChildTasksFromPlans(ctx, qtx, advancedRun, sourceTask, attemptID, childPlans, "planning_intent.replan")
	if err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	if expandEvent.ID.Valid {
		lastEvent = expandEvent
	}

	for _, task := range allTasks {
		if _, ok := subtreeTaskIDs[task.ID.String()]; !ok {
			continue
		}
		if task.SupersededByPlannerEpoch.Valid {
			continue
		}
		supersededTask, err := qtx.MarkOrchestrationTaskSuperseded(ctx, sqlc.MarkOrchestrationTaskSupersededParams{
			ID:                       task.ID,
			SupersededByPlannerEpoch: newPlannerEpochValue,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return sqlc.OrchestrationEvent{}, fmt.Errorf("mark task superseded: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           supersededTask.ID,
			AttemptID:        attemptID,
			AggregateType:    "task",
			AggregateID:      supersededTask.ID,
			AggregateVersion: supersededTask.StatusVersion,
			Type:             "run.event.task.superseded",
			Payload: map[string]any{
				"task_id":                     supersededTask.ID.String(),
				"source_task_id":              sourceTask.ID.String(),
				"planning_intent_id":          intent.ID.String(),
				"status":                      supersededTask.Status,
				"planner_epoch":               supersededTask.PlannerEpoch,
				"superseded_by_planner_epoch": newPlannerEpoch,
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}

	for _, dep := range allDependencies {
		if dep.SupersededByPlannerEpoch.Valid {
			continue
		}
		_, predecessorInSubtree := subtreeTaskIDs[dep.PredecessorTaskID.String()]
		_, successorInSubtree := subtreeTaskIDs[dep.SuccessorTaskID.String()]
		if !predecessorInSubtree && !successorInSubtree {
			continue
		}
		if _, err := qtx.MarkOrchestrationTaskDependencySuperseded(ctx, sqlc.MarkOrchestrationTaskDependencySupersededParams{
			ID:                       dep.ID,
			SupersededByPlannerEpoch: newPlannerEpochValue,
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("mark task dependency superseded: %w", err)
		}
	}

	for _, checkpoint := range allCheckpoints {
		if checkpoint.Status != CheckpointStatusOpen || checkpoint.SupersededByPlannerEpoch.Valid {
			continue
		}
		if _, ok := subtreeTaskIDs[checkpoint.TaskID.String()]; !ok {
			continue
		}
		supersededCheckpoint, err := qtx.MarkOrchestrationHumanCheckpointSuperseded(ctx, sqlc.MarkOrchestrationHumanCheckpointSupersededParams{
			ID:                       checkpoint.ID,
			SupersededByPlannerEpoch: newPlannerEpochValue,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return sqlc.OrchestrationEvent{}, fmt.Errorf("mark checkpoint superseded: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           supersededCheckpoint.TaskID,
			CheckpointID:     supersededCheckpoint.ID,
			AttemptID:        attemptID,
			AggregateType:    "checkpoint",
			AggregateID:      supersededCheckpoint.ID,
			AggregateVersion: supersededCheckpoint.StatusVersion,
			Type:             "run.event.hitl.superseded",
			Payload: map[string]any{
				"checkpoint_id":               supersededCheckpoint.ID.String(),
				"task_id":                     supersededCheckpoint.TaskID.String(),
				"source_task_id":              sourceTask.ID.String(),
				"planning_intent_id":          intent.ID.String(),
				"status":                      supersededCheckpoint.Status,
				"superseded_by_planner_epoch": newPlannerEpoch,
				"blocks_run":                  supersededCheckpoint.BlocksRun,
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
		if supersededCheckpoint.BlocksRun {
			lastEvent, err = s.releaseRunBarrierAfterCheckpointClosure(ctx, qtx, advancedRun, supersededCheckpoint, attemptID, "planning_intent.replan")
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
		}
	}

	return lastEvent, nil
}

func (s *Service) DispatchNextReadyTask(ctx context.Context) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin scheduler tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	activated, err := s.activateDependencySatisfiedTasks(ctx, qtx)
	if err != nil {
		return false, err
	}

	candidates, err := qtx.ListSchedulableOrchestrationTasks(ctx)
	if err != nil {
		return false, fmt.Errorf("list schedulable tasks: %w", err)
	}
	if len(candidates) == 0 {
		if activated {
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit dependency activation tx: %w", err)
			}
			return true, nil
		}
		return false, nil
	}

	var selectedTask sqlc.OrchestrationTask
	var createdAttempt sqlc.OrchestrationTaskAttempt
	for _, candidate := range candidates {
		runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, candidate.RunID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return false, fmt.Errorf("lock run for schedulable task: %w", err)
		}
		if runRow.LifecycleStatus != LifecycleStatusRunning {
			continue
		}
		taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, candidate.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return false, fmt.Errorf("lock schedulable task: %w", err)
		}
		if taskRow.Status != TaskStatusReady || taskRow.SupersededByPlannerEpoch.Valid {
			continue
		}
		if taskRow.RunID != runRow.ID {
			continue
		}
		activeAttempts, err := qtx.CountActiveOrchestrationTaskAttemptsByTask(ctx, taskRow.ID)
		if err != nil {
			return false, fmt.Errorf("count active attempts for task: %w", err)
		}
		if activeAttempts > 0 {
			continue
		}
		manifestHash, err := hashJSON(map[string]any{
			"task_id": taskRow.ID.String(),
			"inputs":  decodeJSONObject(taskRow.Inputs),
		})
		if err != nil {
			return false, fmt.Errorf("hash input manifest: %w", err)
		}
		_, manifestUUID, err := newPGUUID()
		if err != nil {
			return false, err
		}
		manifestRow, err := qtx.CreateOrchestrationInputManifest(ctx, sqlc.CreateOrchestrationInputManifestParams{
			ID:                          manifestUUID,
			RunID:                       taskRow.RunID,
			TaskID:                      taskRow.ID,
			CapturedTaskInputs:          taskRow.Inputs,
			CapturedArtifactVersions:    marshalJSON([]map[string]any{}),
			CapturedBlackboardRevisions: marshalJSON([]map[string]any{}),
			ProjectionHash:              manifestHash,
		})
		if err != nil {
			return false, fmt.Errorf("create input manifest: %w", err)
		}
		attemptNo, err := qtx.GetNextOrchestrationTaskAttemptNo(ctx, taskRow.ID)
		if err != nil {
			return false, fmt.Errorf("get next task attempt number: %w", err)
		}
		_, attemptUUID, err := newPGUUID()
		if err != nil {
			return false, err
		}
		createdAttempt, err = qtx.CreateOrchestrationTaskAttempt(ctx, sqlc.CreateOrchestrationTaskAttemptParams{
			ID:               attemptUUID,
			RunID:            taskRow.RunID,
			TaskID:           taskRow.ID,
			AttemptNo:        attemptNo,
			Status:           TaskAttemptStatusCreated,
			InputManifestID:  manifestRow.ID,
			ParkCheckpointID: pgtype.UUID{},
		})
		if err != nil {
			return false, fmt.Errorf("create task attempt: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, taskRow.RunID, eventSpec{
			TaskID:           createdAttempt.TaskID,
			AttemptID:        createdAttempt.ID,
			AggregateType:    "attempt",
			AggregateID:      createdAttempt.ID,
			AggregateVersion: createdAttempt.ClaimEpoch,
			Type:             "run.event.attempt.created",
			Payload: map[string]any{
				"attempt_id":        createdAttempt.ID.String(),
				"task_id":           createdAttempt.TaskID.String(),
				"run_id":            createdAttempt.RunID.String(),
				"attempt_no":        createdAttempt.AttemptNo,
				"status":            createdAttempt.Status,
				"input_manifest_id": manifestRow.ID.String(),
			},
		}); err != nil {
			return false, err
		}
		dispatchingTask, err := qtx.MarkOrchestrationTaskDispatching(ctx, taskRow.ID)
		if err != nil {
			return false, fmt.Errorf("mark task dispatching: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, taskRow.RunID, eventSpec{
			TaskID:           dispatchingTask.ID,
			AttemptID:        createdAttempt.ID,
			AggregateType:    "task",
			AggregateID:      dispatchingTask.ID,
			AggregateVersion: dispatchingTask.StatusVersion,
			Type:             "run.event.task.dispatching",
			Payload: map[string]any{
				"task_id":         dispatchingTask.ID.String(),
				"previous_status": taskRow.Status,
				"new_status":      dispatchingTask.Status,
				"attempt_id":      createdAttempt.ID.String(),
			},
		}); err != nil {
			return false, err
		}
		selectedTask = dispatchingTask
		break
	}

	if !selectedTask.ID.Valid {
		if activated {
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit dependency activation tx: %w", err)
			}
			return true, nil
		}
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit scheduler tx: %w", err)
	}
	if err := s.attemptAssignments.PublishAttemptAssignment(ctx, toTaskAttempt(createdAttempt)); err != nil {
		s.logger.Warn("publish attempt assignment failed", slog.String("attempt_id", createdAttempt.ID.String()), slog.Any("error", err))
	}
	return true, nil
}

func (s *Service) ClaimNextAttempt(ctx context.Context, claim AttemptClaim) (*TaskAttempt, error) {
	workerID := strings.TrimSpace(claim.WorkerID)
	if workerID == "" {
		return nil, fmt.Errorf("%w: worker_id is required", ErrInvalidArgument)
	}
	executorID := strings.TrimSpace(claim.ExecutorID)
	if executorID == "" {
		executorID = DefaultWorkerExecutorID
	}
	ttl := normalizeLeaseTTL(claim.LeaseTTLSeconds, TaskAttemptDefaultLeaseTTL)
	supportedProfiles := normalizeWorkerProfiles(claim.WorkerProfiles)
	if len(supportedProfiles) == 0 {
		return nil, fmt.Errorf("%w: worker_profiles is required", ErrInvalidArgument)
	}
	leaseToken := strings.TrimSpace(claim.LeaseToken)

	lease, err := s.RegisterWorker(ctx, WorkerRegistration{
		WorkerID:        workerID,
		ExecutorID:      executorID,
		DisplayName:     workerID,
		Capabilities:    workerCapabilities(supportedProfiles),
		LeaseToken:      leaseToken,
		LeaseTTLSeconds: int(ttl / time.Second),
	})
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin attempt claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	attemptRow, err := qtx.ClaimNextCreatedOrchestrationTaskAttempt(ctx, sqlc.ClaimNextCreatedOrchestrationTaskAttemptParams{
		WorkerID:         workerID,
		ExecutorID:       executorID,
		WorkerLeaseToken: lease.LeaseToken,
		WorkerProfiles:   supportedProfiles,
		ClaimToken:       uuid.NewString(),
		LeaseExpiresAt:   timeToPg(time.Now().Add(ttl)),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoRunnableAttempt
		}
		return nil, fmt.Errorf("claim task attempt: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, attemptRow.RunID, eventSpec{
		TaskID:           attemptRow.TaskID,
		AttemptID:        attemptRow.ID,
		AggregateType:    "attempt",
		AggregateID:      attemptRow.ID,
		AggregateVersion: attemptRow.ClaimEpoch,
		Type:             "run.event.attempt.claimed",
		Payload: map[string]any{
			"attempt_id":       attemptRow.ID.String(),
			"task_id":          attemptRow.TaskID.String(),
			"status":           attemptRow.Status,
			"claim_epoch":      attemptRow.ClaimEpoch,
			"claim_token":      attemptRow.ClaimToken,
			"worker_id":        attemptRow.WorkerID,
			"executor_id":      attemptRow.ExecutorID,
			"lease_expires_at": timeForJSON(db.TimeFromPg(attemptRow.LeaseExpiresAt)),
		},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit attempt claim tx: %w", err)
	}
	attempt := toTaskAttempt(attemptRow)
	return &attempt, nil
}

func (s *Service) StartAttempt(ctx context.Context, attemptID, claimToken string) (*TaskAttempt, error) {
	pgAttemptID, err := db.ParseUUID(attemptID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid attempt id", ErrInvalidArgument)
	}
	normalizedClaimToken := strings.TrimSpace(claimToken)
	if normalizedClaimToken == "" {
		return nil, fmt.Errorf("%w: claim token is required", ErrInvalidArgument)
	}

	if err := s.ensureAttemptBinding(ctx, pgAttemptID, normalizedClaimToken); err != nil {
		return nil, err
	}
	return s.advanceAttemptToRunning(ctx, pgAttemptID, normalizedClaimToken)
}

func (s *Service) ensureAttemptBinding(ctx context.Context, attemptID pgtype.UUID, claimToken string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin attempt binding tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	attemptRow, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, attemptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAttemptNotFound
		}
		return fmt.Errorf("lock task attempt for binding: %w", err)
	}
	if strings.TrimSpace(attemptRow.ClaimToken) != claimToken {
		return ErrAttemptLeaseConflict
	}
	if leaseExpired(attemptRow.LeaseExpiresAt) {
		return ErrAttemptLeaseConflict
	}
	if ok, err := ensureWorkerLeaseSnapshot(ctx, qtx, attemptRow.WorkerID, attemptRow.ExecutorID, attemptRow.WorkerLeaseToken); err != nil {
		return err
	} else if !ok {
		return ErrAttemptLeaseConflict
	}
	switch attemptRow.Status {
	case TaskAttemptStatusBinding, TaskAttemptStatusRunning:
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit replayed attempt binding tx: %w", err)
		}
		return nil
	case TaskAttemptStatusClaimed:
	default:
		return ErrAttemptImmutable
	}

	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, attemptRow.RunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunNotFound
		}
		return fmt.Errorf("lock run for attempt binding: %w", err)
	}
	taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, attemptRow.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("lock task for attempt binding: %w", err)
	}
	if runRow.LifecycleStatus != LifecycleStatusRunning || taskRow.Status != TaskStatusDispatching || taskRow.WaitingCheckpointID.Valid || taskRow.SupersededByPlannerEpoch.Valid {
		if _, retireErr := s.retireAttempt(ctx, qtx, attemptRow, attemptRetirementSpec{
			Status:         TaskAttemptStatusFailed,
			FailureClass:   "attempt_immutable",
			TerminalReason: "attempt invalidated before start",
		}); retireErr != nil {
			if errors.Is(retireErr, pgx.ErrNoRows) {
				return ErrAttemptLeaseConflict
			}
			return fmt.Errorf("retire invalid attempt before binding: %w", retireErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit retired invalid attempt before binding: %w", err)
		}
		return ErrAttemptImmutable
	}

	bindingAttempt, err := qtx.MarkOrchestrationTaskAttemptBinding(ctx, sqlc.MarkOrchestrationTaskAttemptBindingParams{
		ID:         attemptRow.ID,
		ClaimToken: claimToken,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAttemptLeaseConflict
		}
		return fmt.Errorf("mark attempt binding: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, bindingAttempt.RunID, eventSpec{
		TaskID:           bindingAttempt.TaskID,
		AttemptID:        bindingAttempt.ID,
		AggregateType:    "attempt",
		AggregateID:      bindingAttempt.ID,
		AggregateVersion: bindingAttempt.ClaimEpoch,
		Type:             "run.event.attempt.binding",
		Payload: map[string]any{
			"attempt_id":       bindingAttempt.ID.String(),
			"task_id":          bindingAttempt.TaskID.String(),
			"previous_status":  attemptRow.Status,
			"new_status":       bindingAttempt.Status,
			"claim_epoch":      bindingAttempt.ClaimEpoch,
			"claim_token":      bindingAttempt.ClaimToken,
			"lease_expires_at": timeForJSON(db.TimeFromPg(bindingAttempt.LeaseExpiresAt)),
		},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit attempt binding tx: %w", err)
	}
	return nil
}

func (s *Service) advanceAttemptToRunning(ctx context.Context, attemptID pgtype.UUID, claimToken string) (*TaskAttempt, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin start attempt tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	attemptRow, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, attemptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttemptNotFound
		}
		return nil, fmt.Errorf("lock task attempt: %w", err)
	}
	if strings.TrimSpace(attemptRow.ClaimToken) != claimToken {
		return nil, ErrAttemptLeaseConflict
	}
	if leaseExpired(attemptRow.LeaseExpiresAt) {
		return nil, ErrAttemptLeaseConflict
	}
	if ok, err := ensureWorkerLeaseSnapshot(ctx, qtx, attemptRow.WorkerID, attemptRow.ExecutorID, attemptRow.WorkerLeaseToken); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAttemptLeaseConflict
	}
	if attemptRow.Status == TaskAttemptStatusRunning {
		taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, attemptRow.TaskID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrTaskNotFound
			}
			return nil, fmt.Errorf("lock task for replayed start attempt: %w", err)
		}
		if taskRow.Status != TaskStatusRunning || taskRow.SupersededByPlannerEpoch.Valid {
			return nil, ErrAttemptImmutable
		}
		attempt := toTaskAttempt(attemptRow)
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed start attempt tx: %w", err)
		}
		return &attempt, nil
	}
	if attemptRow.Status != TaskAttemptStatusBinding {
		return nil, ErrAttemptImmutable
	}

	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, attemptRow.RunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for start attempt: %w", err)
	}
	taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, attemptRow.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock task for start attempt: %w", err)
	}
	if runRow.LifecycleStatus != LifecycleStatusRunning || taskRow.Status != TaskStatusDispatching || taskRow.WaitingCheckpointID.Valid || taskRow.SupersededByPlannerEpoch.Valid {
		if _, retireErr := s.retireAttempt(ctx, qtx, attemptRow, attemptRetirementSpec{
			Status:         TaskAttemptStatusFailed,
			FailureClass:   "attempt_immutable",
			TerminalReason: "attempt invalidated before start",
		}); retireErr != nil {
			if errors.Is(retireErr, pgx.ErrNoRows) {
				return nil, ErrAttemptLeaseConflict
			}
			return nil, fmt.Errorf("retire invalid bound attempt before running: %w", retireErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit retired invalid bound attempt before running: %w", err)
		}
		return nil, ErrAttemptImmutable
	}

	runningAttempt, err := qtx.MarkOrchestrationTaskAttemptRunning(ctx, sqlc.MarkOrchestrationTaskAttemptRunningParams{
		ID:         attemptRow.ID,
		ClaimToken: claimToken,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttemptLeaseConflict
		}
		return nil, fmt.Errorf("mark attempt running: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, runningAttempt.RunID, eventSpec{
		TaskID:           runningAttempt.TaskID,
		AttemptID:        runningAttempt.ID,
		AggregateType:    "attempt",
		AggregateID:      runningAttempt.ID,
		AggregateVersion: runningAttempt.ClaimEpoch,
		Type:             "run.event.attempt.running",
		Payload: map[string]any{
			"attempt_id":       runningAttempt.ID.String(),
			"task_id":          runningAttempt.TaskID.String(),
			"previous_status":  attemptRow.Status,
			"new_status":       runningAttempt.Status,
			"claim_epoch":      runningAttempt.ClaimEpoch,
			"claim_token":      runningAttempt.ClaimToken,
			"lease_expires_at": timeForJSON(db.TimeFromPg(runningAttempt.LeaseExpiresAt)),
		},
	}); err != nil {
		return nil, err
	}
	runningTask, err := qtx.MarkOrchestrationTaskRunning(ctx, taskRow.ID)
	if err != nil {
		return nil, fmt.Errorf("mark task running: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, runningAttempt.RunID, eventSpec{
		TaskID:           runningTask.ID,
		AttemptID:        runningAttempt.ID,
		AggregateType:    "task",
		AggregateID:      runningTask.ID,
		AggregateVersion: runningTask.StatusVersion,
		Type:             "run.event.task.running",
		Payload: map[string]any{
			"task_id":         runningTask.ID.String(),
			"attempt_id":      runningAttempt.ID.String(),
			"previous_status": taskRow.Status,
			"new_status":      runningTask.Status,
		},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit start attempt tx: %w", err)
	}
	attempt := toTaskAttempt(runningAttempt)
	return &attempt, nil
}

func (s *Service) HeartbeatAttempt(ctx context.Context, input AttemptHeartbeat) (*TaskAttempt, error) {
	pgAttemptID, err := db.ParseUUID(input.AttemptID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid attempt id", ErrInvalidArgument)
	}
	normalizedClaimToken := strings.TrimSpace(input.ClaimToken)
	if normalizedClaimToken == "" {
		return nil, fmt.Errorf("%w: claim token is required", ErrInvalidArgument)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin attempt heartbeat tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	attemptRow, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, pgAttemptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttemptNotFound
		}
		return nil, fmt.Errorf("lock task attempt for heartbeat: %w", err)
	}
	if strings.TrimSpace(attemptRow.ClaimToken) != normalizedClaimToken {
		return nil, ErrAttemptLeaseConflict
	}
	if attemptRow.Status != TaskAttemptStatusClaimed &&
		attemptRow.Status != TaskAttemptStatusBinding &&
		attemptRow.Status != TaskAttemptStatusRunning {
		return nil, ErrAttemptImmutable
	}
	if leaseExpired(attemptRow.LeaseExpiresAt) {
		return nil, ErrAttemptLeaseConflict
	}
	if ok, err := ensureWorkerLeaseSnapshot(ctx, qtx, attemptRow.WorkerID, attemptRow.ExecutorID, attemptRow.WorkerLeaseToken); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAttemptLeaseConflict
	}
	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, attemptRow.RunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for attempt heartbeat: %w", err)
	}
	if runRow.LifecycleStatus != LifecycleStatusRunning {
		return nil, ErrAttemptImmutable
	}
	taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, attemptRow.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock task for attempt heartbeat: %w", err)
	}
	switch attemptRow.Status {
	case TaskAttemptStatusClaimed:
		if taskRow.Status != TaskStatusDispatching || taskRow.WaitingCheckpointID.Valid || taskRow.SupersededByPlannerEpoch.Valid {
			return nil, ErrAttemptImmutable
		}
	case TaskAttemptStatusBinding:
		if taskRow.Status != TaskStatusDispatching || taskRow.WaitingCheckpointID.Valid || taskRow.SupersededByPlannerEpoch.Valid {
			return nil, ErrAttemptImmutable
		}
	case TaskAttemptStatusRunning:
		if taskRow.Status != TaskStatusRunning || taskRow.SupersededByPlannerEpoch.Valid {
			return nil, ErrAttemptImmutable
		}
	}
	row, err := qtx.HeartbeatOrchestrationTaskAttempt(ctx, sqlc.HeartbeatOrchestrationTaskAttemptParams{
		ID:             pgAttemptID,
		ClaimToken:     normalizedClaimToken,
		LeaseExpiresAt: timeToPg(time.Now().Add(normalizeLeaseTTL(input.LeaseTTLSeconds, TaskAttemptDefaultLeaseTTL))),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttemptLeaseConflict
		}
		return nil, fmt.Errorf("heartbeat task attempt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit attempt heartbeat tx: %w", err)
	}
	attempt := toTaskAttempt(row)
	return &attempt, nil
}

func (s *Service) CompleteAttempt(ctx context.Context, input AttemptCompletion) (*TaskAttempt, error) {
	pgAttemptID, err := db.ParseUUID(input.AttemptID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid attempt id", ErrInvalidArgument)
	}
	normalizedClaimToken := strings.TrimSpace(input.ClaimToken)
	if normalizedClaimToken == "" {
		return nil, fmt.Errorf("%w: claim token is required", ErrInvalidArgument)
	}
	completionStatus := normalizeAttemptCompletionStatus(input.Status)
	if completionStatus == "" {
		return nil, fmt.Errorf("%w: unsupported attempt completion status %q", ErrInvalidArgument, input.Status)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin complete attempt tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	attemptRow, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, pgAttemptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttemptNotFound
		}
		return nil, fmt.Errorf("lock task attempt for completion: %w", err)
	}
	if strings.TrimSpace(attemptRow.ClaimToken) != normalizedClaimToken {
		return nil, ErrAttemptLeaseConflict
	}
	if attemptRow.Status == TaskAttemptStatusLost {
		return nil, ErrAttemptLeaseConflict
	}
	if isTerminalAttemptStatus(attemptRow.Status) {
		attempt := toTaskAttempt(attemptRow)
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit replayed complete attempt tx: %w", err)
		}
		return &attempt, nil
	}
	if leaseExpired(attemptRow.LeaseExpiresAt) {
		return nil, ErrAttemptLeaseConflict
	}
	if attemptRow.Status != TaskAttemptStatusRunning {
		return nil, ErrAttemptImmutable
	}
	if ok, err := ensureWorkerLeaseSnapshot(ctx, qtx, attemptRow.WorkerID, attemptRow.ExecutorID, attemptRow.WorkerLeaseToken); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAttemptLeaseConflict
	}

	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, attemptRow.RunID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("lock run for attempt completion: %w", err)
	}
	if runRow.LifecycleStatus != LifecycleStatusRunning {
		return nil, ErrAttemptImmutable
	}
	taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, attemptRow.TaskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("lock task for attempt completion: %w", err)
	}
	if taskRow.Status != TaskStatusRunning || taskRow.SupersededByPlannerEpoch.Valid {
		return nil, ErrAttemptImmutable
	}

	resultRow, err := qtx.CreateOrchestrationTaskResult(ctx, sqlc.CreateOrchestrationTaskResultParams{
		RunID:            attemptRow.RunID,
		TaskID:           attemptRow.TaskID,
		AttemptID:        attemptRow.ID,
		Status:           completionStatus,
		Summary:          strings.TrimSpace(input.Summary),
		FailureClass:     strings.TrimSpace(input.FailureClass),
		RequestReplan:    input.RequestReplan,
		ArtifactIntents:  marshalJSON(input.ArtifactIntents),
		StructuredOutput: marshalObject(input.StructuredOutput),
	})
	if err != nil {
		return nil, fmt.Errorf("create task result from attempt completion: %w", err)
	}

	for _, artifact := range input.ArtifactIntents {
		if strings.TrimSpace(artifact.Kind) == "" || strings.TrimSpace(artifact.URI) == "" || strings.TrimSpace(artifact.Version) == "" || strings.TrimSpace(artifact.Digest) == "" {
			continue
		}
		artifactID, artifactUUID, err := newPGUUID()
		if err != nil {
			return nil, err
		}
		artifactRow, err := qtx.CreateOrchestrationArtifact(ctx, sqlc.CreateOrchestrationArtifactParams{
			ID:          artifactUUID,
			RunID:       attemptRow.RunID,
			TaskID:      attemptRow.TaskID,
			AttemptID:   attemptRow.ID,
			Kind:        strings.TrimSpace(artifact.Kind),
			Uri:         strings.TrimSpace(artifact.URI),
			Version:     strings.TrimSpace(artifact.Version),
			Digest:      strings.TrimSpace(artifact.Digest),
			ContentType: strings.TrimSpace(artifact.ContentType),
			Summary:     strings.TrimSpace(artifact.Summary),
			Metadata:    marshalObject(artifact.Metadata),
		})
		if err != nil {
			return nil, fmt.Errorf("create artifact from attempt completion: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, attemptRow.RunID, eventSpec{
			TaskID:           attemptRow.TaskID,
			AttemptID:        attemptRow.ID,
			AggregateType:    "artifact",
			AggregateID:      artifactUUID,
			AggregateVersion: 1,
			Type:             "run.event.artifact.committed",
			Payload: map[string]any{
				"artifact_id":  artifactID,
				"task_id":      attemptRow.TaskID.String(),
				"attempt_id":   attemptRow.ID.String(),
				"kind":         artifactRow.Kind,
				"uri":          artifactRow.Uri,
				"version":      artifactRow.Version,
				"digest":       artifactRow.Digest,
				"content_type": artifactRow.ContentType,
				"summary":      artifactRow.Summary,
				"metadata":     normalizeObject(artifact.Metadata),
			},
		}); err != nil {
			return nil, err
		}
	}

	var finalAttempt sqlc.OrchestrationTaskAttempt
	switch completionStatus {
	case TaskAttemptStatusCompleted:
		finalAttempt, err = qtx.MarkOrchestrationTaskAttemptCompleted(ctx, sqlc.MarkOrchestrationTaskAttemptCompletedParams{
			ID:         attemptRow.ID,
			ClaimToken: normalizedClaimToken,
		})
		if err != nil {
			return nil, fmt.Errorf("mark attempt completed: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, attemptRow.RunID, eventSpec{
			TaskID:           finalAttempt.TaskID,
			AttemptID:        finalAttempt.ID,
			AggregateType:    "attempt",
			AggregateID:      finalAttempt.ID,
			AggregateVersion: finalAttempt.ClaimEpoch,
			Type:             "run.event.attempt.completed",
			Payload: map[string]any{
				"attempt_id":       finalAttempt.ID.String(),
				"task_id":          finalAttempt.TaskID.String(),
				"previous_status":  attemptRow.Status,
				"new_status":       finalAttempt.Status,
				"summary":          resultRow.Summary,
				"request_replan":   resultRow.RequestReplan,
				"lease_expires_at": timeForJSON(db.TimeFromPg(finalAttempt.LeaseExpiresAt)),
			},
		}); err != nil {
			return nil, err
		}
	case TaskAttemptStatusFailed:
		finalAttempt, err = qtx.MarkOrchestrationTaskAttemptFailed(ctx, sqlc.MarkOrchestrationTaskAttemptFailedParams{
			ID:             attemptRow.ID,
			ClaimToken:     normalizedClaimToken,
			FailureClass:   strings.TrimSpace(input.FailureClass),
			TerminalReason: normalizeAttemptTerminalReason(input.TerminalReason),
		})
		if err != nil {
			return nil, fmt.Errorf("mark attempt failed: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, attemptRow.RunID, eventSpec{
			TaskID:           finalAttempt.TaskID,
			AttemptID:        finalAttempt.ID,
			AggregateType:    "attempt",
			AggregateID:      finalAttempt.ID,
			AggregateVersion: finalAttempt.ClaimEpoch,
			Type:             "run.event.attempt.failed",
			Payload: map[string]any{
				"attempt_id":      finalAttempt.ID.String(),
				"task_id":         finalAttempt.TaskID.String(),
				"previous_status": attemptRow.Status,
				"new_status":      finalAttempt.Status,
				"failure_class":   finalAttempt.FailureClass,
				"terminal_reason": finalAttempt.TerminalReason,
			},
		}); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unsupported attempt completion status %q", ErrInvalidArgument, completionStatus)
	}

	_, planningIntentUUID, err := newPGUUID()
	if err != nil {
		return nil, err
	}
	planningIntent, err := qtx.CreateOrchestrationPlanningIntent(ctx, sqlc.CreateOrchestrationPlanningIntentParams{
		ID:               planningIntentUUID,
		RunID:            attemptRow.RunID,
		TaskID:           attemptRow.TaskID,
		CheckpointID:     pgtype.UUID{},
		Kind:             PlanningIntentKindAttemptFinalize,
		Status:           PlanningIntentStatusPending,
		BasePlannerEpoch: runRow.PlannerEpoch,
		Payload: marshalJSON(map[string]any{
			"run_id":            attemptRow.RunID.String(),
			"task_id":           attemptRow.TaskID.String(),
			"attempt_id":        attemptRow.ID.String(),
			"result_id":         resultRow.ID.String(),
			"completion_status": completionStatus,
			"failure_class":     strings.TrimSpace(input.FailureClass),
			"terminal_reason":   normalizeAttemptTerminalReason(input.TerminalReason),
			"structured_output": normalizeObject(input.StructuredOutput),
			"request_replan":    resultRow.RequestReplan,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("create attempt finalize planning intent: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, attemptRow.RunID, eventSpec{
		TaskID:           attemptRow.TaskID,
		AttemptID:        attemptRow.ID,
		AggregateType:    "planning_intent",
		AggregateID:      planningIntent.ID,
		AggregateVersion: 1,
		Type:             "run.event.planning_intent.enqueued",
		Payload: map[string]any{
			"planning_intent_id": planningIntent.ID.String(),
			"run_id":             attemptRow.RunID.String(),
			"task_id":            attemptRow.TaskID.String(),
			"attempt_id":         attemptRow.ID.String(),
			"kind":               planningIntent.Kind,
			"status":             planningIntent.Status,
		},
	}); err != nil {
		return nil, err
	}
	if err := s.syncRunPlanningStatus(ctx, qtx, runRow.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit complete attempt tx: %w", err)
	}
	attempt := toTaskAttempt(finalAttempt)
	return &attempt, nil
}

func (s *Service) RecoverExpiredAttempts(ctx context.Context) (int, error) {
	expiredAttempts, err := s.queries.ListExpiredOrchestrationTaskAttempts(ctx)
	if err != nil {
		return 0, fmt.Errorf("list expired attempts: %w", err)
	}
	recovered := 0
	for _, candidate := range expiredAttempts {
		tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return recovered, fmt.Errorf("begin attempt recovery tx: %w", err)
		}
		qtx := s.queries.WithTx(tx)
		attemptRow, err := qtx.GetOrchestrationTaskAttemptByIDForUpdate(ctx, candidate.ID)
		if err != nil {
			_ = tx.Rollback(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return recovered, fmt.Errorf("lock expired attempt: %w", err)
		}
		if isTerminalAttemptStatus(attemptRow.Status) || !leaseExpired(attemptRow.LeaseExpiresAt) {
			_ = tx.Rollback(ctx)
			continue
		}
		runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, attemptRow.RunID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return recovered, fmt.Errorf("lock run for expired attempt: %w", err)
		}
		taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, attemptRow.TaskID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return recovered, fmt.Errorf("lock task for expired attempt: %w", err)
		}
		if attemptRow.Status == TaskAttemptStatusClaimed || attemptRow.Status == TaskAttemptStatusBinding {
			requeueReason := "attempt lease expired before execution start"
			if attemptRow.Status == TaskAttemptStatusBinding {
				requeueReason = "attempt binding lease expired before execution start"
			}
			releasedAttempt, err := qtx.ReleaseOrchestrationTaskAttemptClaim(ctx, sqlc.ReleaseOrchestrationTaskAttemptClaimParams{
				ID:         attemptRow.ID,
				ClaimToken: attemptRow.ClaimToken,
			})
			if err != nil {
				_ = tx.Rollback(ctx)
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return recovered, fmt.Errorf("release expired binding attempt: %w", err)
			}
			if _, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
				TaskID:           releasedAttempt.TaskID,
				AttemptID:        releasedAttempt.ID,
				AggregateType:    "attempt",
				AggregateID:      releasedAttempt.ID,
				AggregateVersion: releasedAttempt.ClaimEpoch,
				Type:             "run.event.attempt.requeued",
				Payload: map[string]any{
					"attempt_id":      releasedAttempt.ID.String(),
					"task_id":         releasedAttempt.TaskID.String(),
					"previous_status": attemptRow.Status,
					"new_status":      releasedAttempt.Status,
					"terminal_reason": requeueReason,
				},
			}); err != nil {
				_ = tx.Rollback(ctx)
				return recovered, err
			}
			if err := tx.Commit(ctx); err != nil {
				return recovered, fmt.Errorf("commit pre-start attempt requeue tx: %w", err)
			}
			recovered++
			continue
		}
		lostAttempt, err := s.retireAttempt(ctx, qtx, attemptRow, attemptRetirementSpec{
			Status:         TaskAttemptStatusLost,
			TerminalReason: "attempt lease expired",
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			return recovered, fmt.Errorf("retire expired attempt: %w", err)
		}
		resultRow, err := qtx.CreateOrchestrationTaskResult(ctx, sqlc.CreateOrchestrationTaskResultParams{
			RunID:            attemptRow.RunID,
			TaskID:           attemptRow.TaskID,
			AttemptID:        attemptRow.ID,
			Status:           TaskAttemptStatusFailed,
			Summary:          "attempt lease expired",
			FailureClass:     "lease_expired",
			RequestReplan:    false,
			ArtifactIntents:  marshalJSON([]AttemptArtifactIntent{}),
			StructuredOutput: marshalObject(nil),
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			return recovered, fmt.Errorf("create result for lost attempt: %w", err)
		}
		if taskRow.SupersededByPlannerEpoch.Valid {
			if _, _, err := s.maybeMarkRunCompletedAfterTaskTransition(ctx, qtx, runRow, taskRow.ID, lostAttempt.ID); err != nil {
				_ = tx.Rollback(ctx)
				return recovered, err
			}
			if err := tx.Commit(ctx); err != nil {
				return recovered, fmt.Errorf("commit superseded attempt recovery tx: %w", err)
			}
			recovered++
			continue
		}
		failedTask, err := qtx.MarkOrchestrationTaskFailed(ctx, sqlc.MarkOrchestrationTaskFailedParams{
			ID:             taskRow.ID,
			LatestResultID: resultRow.ID,
			TerminalReason: "attempt lease expired",
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			return recovered, fmt.Errorf("mark task failed from lost attempt: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           failedTask.ID,
			AttemptID:        lostAttempt.ID,
			AggregateType:    "task",
			AggregateID:      failedTask.ID,
			AggregateVersion: failedTask.StatusVersion,
			Type:             "run.event.task.failed",
			Payload: map[string]any{
				"task_id":          failedTask.ID.String(),
				"attempt_id":       lostAttempt.ID.String(),
				"previous_status":  taskRow.Status,
				"new_status":       failedTask.Status,
				"latest_result_id": resultRow.ID.String(),
				"failure_class":    "lease_expired",
				"terminal_reason":  failedTask.TerminalReason,
			},
		}); err != nil {
			_ = tx.Rollback(ctx)
			return recovered, err
		}
		if err := s.markRunFailedFromTaskFailure(ctx, qtx, runRow, failedTask, lostAttempt); err != nil {
			_ = tx.Rollback(ctx)
			return recovered, err
		}
		if err := tx.Commit(ctx); err != nil {
			return recovered, fmt.Errorf("commit attempt recovery tx: %w", err)
		}
		recovered++
	}
	return recovered, nil
}

func (s *Service) RunPlannerLoop(ctx context.Context) {
	runBoolLoop(ctx, s.logger, "planner", 200*time.Millisecond, s.ProcessNextPlanningIntent)
}

func (s *Service) RunSchedulerLoop(ctx context.Context) {
	runBoolLoop(ctx, s.logger, "scheduler", 200*time.Millisecond, s.DispatchNextReadyTask)
}

func (s *Service) RunRecoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := s.RecoverExpiredAttempts(ctx)
			if err != nil {
				s.logger.Error("attempt recovery loop failed", slog.Any("error", err))
				continue
			}
			if count > 0 {
				s.logger.Info("recovered expired orchestration attempts", slog.Int("count", count))
			}
		}
	}
}

func (s *Service) maybeMarkRunCompletedAfterPlanningIntent(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, intent sqlc.OrchestrationPlanningIntent) (sqlc.OrchestrationEvent, bool, error) {
	payload := decodeJSONObject(intent.Payload)
	attemptID, _ := payload["attempt_id"].(string)
	attemptUUID := pgtype.UUID{}
	var err error
	if strings.TrimSpace(attemptID) != "" {
		attemptUUID, err = db.ParseUUID(attemptID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, false, fmt.Errorf("%w: invalid completed planning intent attempt id", ErrPlanningIntentNotFound)
		}
	}
	return s.maybeMarkRunCompletedAfterTaskTransition(ctx, qtx, runRow, intent.TaskID, attemptUUID)
}

func (s *Service) maybeMarkRunCompletedAfterTaskTransition(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, taskID, attemptID pgtype.UUID) (sqlc.OrchestrationEvent, bool, error) {
	if runRow.LifecycleStatus != LifecycleStatusRunning {
		return sqlc.OrchestrationEvent{}, false, nil
	}
	activeAttempts, err := qtx.CountActiveOrchestrationTaskAttemptsByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, false, fmt.Errorf("count active attempts by run: %w", err)
	}
	activeIntents, err := qtx.CountActiveOrchestrationPlanningIntentsByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, false, fmt.Errorf("count active planning intents by run: %w", err)
	}
	checkpoints, err := qtx.ListCurrentOrchestrationCheckpointsByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, false, fmt.Errorf("list checkpoints for run completion: %w", err)
	}
	openCheckpoints := 0
	for _, checkpoint := range checkpoints {
		if checkpoint.Status == CheckpointStatusOpen {
			openCheckpoints++
		}
	}
	if activeAttempts > 0 || activeIntents > 0 || openCheckpoints > 0 {
		return sqlc.OrchestrationEvent{}, false, nil
	}
	nonTerminalTasks, err := qtx.CountNonTerminalOrchestrationTasksByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, false, fmt.Errorf("count non-terminal tasks by run: %w", err)
	}
	if nonTerminalTasks > 0 {
		return sqlc.OrchestrationEvent{}, false, nil
	}
	failedTasks, err := qtx.CountFailedOrchestrationTasksByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, false, fmt.Errorf("count failed tasks by run: %w", err)
	}
	if failedTasks > 0 {
		return sqlc.OrchestrationEvent{}, false, nil
	}

	completedRun, err := qtx.MarkOrchestrationRunCompleted(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, false, fmt.Errorf("mark run completed: %w", err)
	}
	event, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		TaskID:           taskID,
		AttemptID:        attemptID,
		AggregateType:    "run",
		AggregateID:      completedRun.ID,
		AggregateVersion: completedRun.StatusVersion,
		Type:             "run.event.completed",
		Payload: map[string]any{
			"run_id":          completedRun.ID.String(),
			"previous_status": runRow.LifecycleStatus,
			"new_status":      completedRun.LifecycleStatus,
			"task_id":         uuidString(taskID),
			"attempt_id":      uuidString(attemptID),
		},
	})
	if err != nil {
		return sqlc.OrchestrationEvent{}, false, err
	}
	return event, true, nil
}

func (s *Service) markRunFailedFromTaskFailure(ctx context.Context, qtx *sqlc.Queries, runRow sqlc.OrchestrationRun, taskRow sqlc.OrchestrationTask, attemptRow sqlc.OrchestrationTaskAttempt) error {
	if runRow.LifecycleStatus != LifecycleStatusRunning {
		return nil
	}
	failedRun, err := qtx.MarkOrchestrationRunFailed(ctx, sqlc.MarkOrchestrationRunFailedParams{
		ID:             runRow.ID,
		TerminalReason: taskRow.TerminalReason,
	})
	if err != nil {
		return fmt.Errorf("mark run failed: %w", err)
	}
	_, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		TaskID:           taskRow.ID,
		AttemptID:        attemptRow.ID,
		AggregateType:    "run",
		AggregateID:      failedRun.ID,
		AggregateVersion: failedRun.StatusVersion,
		Type:             "run.event.failed",
		Payload: map[string]any{
			"run_id":          failedRun.ID.String(),
			"previous_status": runRow.LifecycleStatus,
			"new_status":      failedRun.LifecycleStatus,
			"terminal_reason": failedRun.TerminalReason,
			"task_id":         taskRow.ID.String(),
			"attempt_id":      attemptRow.ID.String(),
		},
	})
	return err
}

func (s *Service) syncRunPlanningStatus(ctx context.Context, qtx *sqlc.Queries, runID pgtype.UUID) error {
	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, runID)
	if err != nil {
		return fmt.Errorf("lock run for planning status sync: %w", err)
	}
	activeIntents, err := qtx.CountActiveOrchestrationPlanningIntentsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("count active planning intents by run: %w", err)
	}
	targetStatus := PlanningStatusIdle
	if activeIntents > 0 {
		targetStatus = PlanningStatusActive
	}
	if runRow.PlanningStatus == targetStatus {
		return nil
	}
	var updatedRun sqlc.OrchestrationRun
	if targetStatus == PlanningStatusActive {
		updatedRun, err = qtx.MarkOrchestrationRunPlanningActive(ctx, runID)
	} else {
		updatedRun, err = qtx.MarkOrchestrationRunPlanningIdle(ctx, runID)
	}
	if err != nil {
		return fmt.Errorf("sync run planning status: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, runID, eventSpec{
		AggregateType:    "run",
		AggregateID:      updatedRun.ID,
		AggregateVersion: updatedRun.StatusVersion,
		Type:             "run.event.planning_status.changed",
		Payload: map[string]any{
			"run_id":                   updatedRun.ID.String(),
			"previous_planning_status": runRow.PlanningStatus,
			"new_planning_status":      updatedRun.PlanningStatus,
		},
	}); err != nil {
		return err
	}
	return nil
}

func runBoolLoop(
	ctx context.Context,
	logger interface{ Error(msg string, args ...any) },
	name string,
	idleDelay time.Duration,
	fn func(context.Context) (bool, error),
) {
	for {
		processed, err := fn(ctx)
		if err != nil {
			logger.Error("orchestration loop failed", slog.String("loop", name), slog.Any("error", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(idleDelay):
			}
			continue
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(idleDelay):
		}
	}
}

func normalizeLeaseTTL(rawSeconds int, fallback time.Duration) time.Duration {
	if rawSeconds <= 0 {
		return fallback
	}
	return time.Duration(rawSeconds) * time.Second
}

func ensureWorkerLeaseSnapshot(ctx context.Context, qtx *sqlc.Queries, workerID, executorID, workerLeaseToken string) (bool, error) {
	workerID = strings.TrimSpace(workerID)
	executorID = strings.TrimSpace(executorID)
	workerLeaseToken = strings.TrimSpace(workerLeaseToken)
	if workerID == "" || executorID == "" || workerLeaseToken == "" {
		return false, nil
	}
	row, err := qtx.GetOrchestrationWorkerByIDForUpdate(ctx, workerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("lock worker lease snapshot: %w", err)
	}
	if strings.TrimSpace(row.ExecutorID) != executorID {
		return false, nil
	}
	if strings.TrimSpace(row.LeaseToken) != workerLeaseToken {
		return false, nil
	}
	if leaseExpired(row.LeaseExpiresAt) {
		return false, nil
	}
	return true, nil
}

func (s *Service) activateDependencySatisfiedTasks(ctx context.Context, qtx *sqlc.Queries) (bool, error) {
	candidates, err := qtx.ListDependencyBlockedOrchestrationTasks(ctx)
	if err != nil {
		return false, fmt.Errorf("list dependency-blocked tasks: %w", err)
	}
	activated := false
	for _, candidate := range candidates {
		deps, err := qtx.ListActiveOrchestrationTaskDependenciesBySuccessor(ctx, candidate.ID)
		if err != nil {
			return false, fmt.Errorf("list task dependencies by successor: %w", err)
		}
		if len(deps) == 0 {
			continue
		}
		taskRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, candidate.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return false, fmt.Errorf("lock dependency-blocked task: %w", err)
		}
		if taskRow.Status != TaskStatusCreated || taskRow.SupersededByPlannerEpoch.Valid {
			continue
		}
		satisfied, err := taskDependenciesSatisfied(ctx, qtx, taskRow.RunID, deps)
		if err != nil {
			return false, err
		}
		if !satisfied {
			continue
		}
		if taskRow.Kind == "join" {
			completed, err := s.completeJoinTask(ctx, qtx, taskRow, deps)
			if err != nil {
				return false, err
			}
			if completed {
				activated = true
			}
			continue
		}
		readyTask, err := qtx.MarkOrchestrationTaskReadyFromCheckpoint(ctx, taskRow.ID)
		if err != nil {
			return false, fmt.Errorf("mark dependency-satisfied task ready: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, readyTask.RunID, eventSpec{
			TaskID:           readyTask.ID,
			AggregateType:    "task",
			AggregateID:      readyTask.ID,
			AggregateVersion: readyTask.StatusVersion,
			Type:             "run.event.task.ready",
			Payload: map[string]any{
				"task_id":             readyTask.ID.String(),
				"previous_status":     taskRow.Status,
				"new_status":          readyTask.Status,
				"ready_reason":        "dependencies_satisfied",
				"depends_on_task_ids": dependencyPredecessorIDs(deps),
			},
		}); err != nil {
			return false, err
		}
		activated = true
	}
	return activated, nil
}

func (s *Service) completeJoinTask(ctx context.Context, qtx *sqlc.Queries, taskRow sqlc.OrchestrationTask, deps []sqlc.OrchestrationTaskDependency) (bool, error) {
	resultRow, err := qtx.CreateOrchestrationTaskResult(ctx, sqlc.CreateOrchestrationTaskResultParams{
		RunID:           taskRow.RunID,
		TaskID:          taskRow.ID,
		AttemptID:       pgtype.UUID{},
		Status:          TaskAttemptStatusCompleted,
		Summary:         fmt.Sprintf("join completed after %d predecessors", len(deps)),
		FailureClass:    "",
		RequestReplan:   false,
		ArtifactIntents: marshalJSON([]AttemptArtifactIntent{}),
		StructuredOutput: marshalObject(map[string]any{
			"join": map[string]any{
				"predecessor_task_ids": dependencyPredecessorIDs(deps),
			},
		}),
	})
	if err != nil {
		return false, fmt.Errorf("create join task result: %w", err)
	}
	completedTask, err := qtx.MarkOrchestrationTaskCompleted(ctx, sqlc.MarkOrchestrationTaskCompletedParams{
		ID:             taskRow.ID,
		LatestResultID: resultRow.ID,
	})
	if err != nil {
		return false, fmt.Errorf("mark join task completed: %w", err)
	}
	if _, err := s.appendEvent(ctx, qtx, taskRow.RunID, eventSpec{
		TaskID:           completedTask.ID,
		AggregateType:    "task",
		AggregateID:      completedTask.ID,
		AggregateVersion: completedTask.StatusVersion,
		Type:             "run.event.task.completed",
		Payload: map[string]any{
			"task_id":           completedTask.ID.String(),
			"previous_status":   taskRow.Status,
			"new_status":        completedTask.Status,
			"latest_result_id":  resultRow.ID.String(),
			"structured_output": map[string]any{"predecessor_task_ids": dependencyPredecessorIDs(deps)},
			"completion_reason": "join_dependencies_satisfied",
		},
	}); err != nil {
		return false, err
	}
	runRow, err := qtx.GetOrchestrationRunByIDForUpdate(ctx, taskRow.RunID)
	if err != nil {
		return false, fmt.Errorf("lock run for join completion: %w", err)
	}
	if completionEvent, completed, err := s.maybeMarkRunCompletedAfterTaskTransition(ctx, qtx, runRow, taskRow.ID, pgtype.UUID{}); err != nil {
		return false, err
	} else if completed {
		_ = completionEvent
	}
	return true, nil
}

func taskDependenciesSatisfied(ctx context.Context, qtx *sqlc.Queries, runID pgtype.UUID, deps []sqlc.OrchestrationTaskDependency) (bool, error) {
	if len(deps) == 0 {
		return true, nil
	}
	for _, dep := range deps {
		depTask, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, dep.PredecessorTaskID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, fmt.Errorf("lock dependency task: %w", err)
		}
		if depTask.RunID != runID || depTask.SupersededByPlannerEpoch.Valid || depTask.Status != TaskStatusCompleted {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) propagateTaskFailureToDependents(ctx context.Context, qtx *sqlc.Queries, failedTask sqlc.OrchestrationTask, attemptRow sqlc.OrchestrationTaskAttempt) error {
	deps, err := qtx.ListActiveOrchestrationTaskDependenciesByPredecessor(ctx, failedTask.ID)
	if err != nil {
		return fmt.Errorf("list task dependencies by predecessor: %w", err)
	}
	for _, dep := range deps {
		successorRow, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, dep.SuccessorTaskID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return fmt.Errorf("lock dependent task: %w", err)
		}
		if successorRow.RunID != failedTask.RunID || successorRow.SupersededByPlannerEpoch.Valid {
			continue
		}
		switch successorRow.Status {
		case TaskStatusCompleted, TaskStatusFailed, TaskStatusBlocked, TaskStatusCancelled:
			continue
		}
		blockedReason := fmt.Sprintf("dependency_failed:%s", failedTask.ID.String())
		blockedTask, err := qtx.MarkOrchestrationTaskBlocked(ctx, sqlc.MarkOrchestrationTaskBlockedParams{
			ID:            successorRow.ID,
			BlockedReason: blockedReason,
		})
		if err != nil {
			return fmt.Errorf("mark dependent task blocked: %w", err)
		}
		if _, err := s.appendEvent(ctx, qtx, failedTask.RunID, eventSpec{
			TaskID:           blockedTask.ID,
			AttemptID:        attemptRow.ID,
			AggregateType:    "task",
			AggregateID:      blockedTask.ID,
			AggregateVersion: blockedTask.StatusVersion,
			Type:             "run.event.task.blocked",
			Payload: map[string]any{
				"task_id":              blockedTask.ID.String(),
				"previous_status":      successorRow.Status,
				"new_status":           blockedTask.Status,
				"blocked_reason":       blockedTask.BlockedReason,
				"failed_dependency_id": failedTask.ID.String(),
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func normalizeAttemptCompletionStatus(raw string) string {
	switch strings.TrimSpace(raw) {
	case "", TaskAttemptStatusCompleted:
		return TaskAttemptStatusCompleted
	case TaskAttemptStatusFailed:
		return TaskAttemptStatusFailed
	default:
		return ""
	}
}

func normalizeAttemptTerminalReason(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "attempt failed"
	}
	return trimmed
}

func leaseExpired(value pgtype.Timestamptz) bool {
	if !value.Valid {
		return false
	}
	return !db.TimeFromPg(value).After(time.Now())
}

func isTerminalAttemptStatus(status string) bool {
	switch status {
	case TaskAttemptStatusCompleted, TaskAttemptStatusFailed, TaskAttemptStatusLost:
		return true
	default:
		return false
	}
}

type attemptRetirementSpec struct {
	Status         string
	FailureClass   string
	TerminalReason string
}

func (s *Service) retireAttempt(ctx context.Context, qtx *sqlc.Queries, attemptRow sqlc.OrchestrationTaskAttempt, spec attemptRetirementSpec) (sqlc.OrchestrationTaskAttempt, error) {
	var (
		finalAttempt sqlc.OrchestrationTaskAttempt
		err          error
		eventType    string
		payload      map[string]any
	)

	switch spec.Status {
	case TaskAttemptStatusFailed:
		finalAttempt, err = qtx.RetireOrchestrationTaskAttemptFailed(ctx, sqlc.RetireOrchestrationTaskAttemptFailedParams{
			ID:             attemptRow.ID,
			ClaimToken:     attemptRow.ClaimToken,
			FailureClass:   strings.TrimSpace(spec.FailureClass),
			TerminalReason: normalizeAttemptTerminalReason(spec.TerminalReason),
		})
		if err != nil {
			return sqlc.OrchestrationTaskAttempt{}, err
		}
		eventType = "run.event.attempt.failed"
		payload = map[string]any{
			"attempt_id":      finalAttempt.ID.String(),
			"task_id":         finalAttempt.TaskID.String(),
			"previous_status": attemptRow.Status,
			"new_status":      finalAttempt.Status,
			"failure_class":   finalAttempt.FailureClass,
			"terminal_reason": finalAttempt.TerminalReason,
		}
	case TaskAttemptStatusLost:
		finalAttempt, err = qtx.MarkOrchestrationTaskAttemptLost(ctx, sqlc.MarkOrchestrationTaskAttemptLostParams{
			ID:             attemptRow.ID,
			ClaimEpoch:     attemptRow.ClaimEpoch,
			TerminalReason: normalizeAttemptTerminalReason(spec.TerminalReason),
		})
		if err != nil {
			return sqlc.OrchestrationTaskAttempt{}, err
		}
		eventType = "run.event.attempt.lost"
		payload = map[string]any{
			"attempt_id":      finalAttempt.ID.String(),
			"task_id":         finalAttempt.TaskID.String(),
			"previous_status": attemptRow.Status,
			"new_status":      finalAttempt.Status,
			"terminal_reason": finalAttempt.TerminalReason,
		}
	default:
		return sqlc.OrchestrationTaskAttempt{}, fmt.Errorf("%w: unsupported attempt retirement status %q", ErrInvalidArgument, spec.Status)
	}

	if _, err := s.appendEvent(ctx, qtx, finalAttempt.RunID, eventSpec{
		TaskID:           finalAttempt.TaskID,
		AttemptID:        finalAttempt.ID,
		AggregateType:    "attempt",
		AggregateID:      finalAttempt.ID,
		AggregateVersion: finalAttempt.ClaimEpoch,
		Type:             eventType,
		Payload:          payload,
	}); err != nil {
		return sqlc.OrchestrationTaskAttempt{}, err
	}

	return finalAttempt, nil
}

func shouldFailPlanningIntent(err error) bool {
	return errors.Is(err, ErrTaskNotFound) ||
		errors.Is(err, ErrRunNotFound) ||
		errors.Is(err, ErrAttemptNotFound) ||
		errors.Is(err, ErrCheckpointNotFound) ||
		errors.Is(err, ErrCheckpointNotOpen) ||
		errors.Is(err, ErrPlanningIntentNotFound) ||
		errors.Is(err, ErrPlanningIntentInvalid)
}

func toTaskAttempt(row sqlc.OrchestrationTaskAttempt) TaskAttempt {
	claimEpoch, _ := uint64FromInt64(row.ClaimEpoch, "claim_epoch")
	return TaskAttempt{
		ID:               row.ID.String(),
		RunID:            row.RunID.String(),
		TaskID:           row.TaskID.String(),
		AttemptNo:        int(row.AttemptNo),
		WorkerID:         row.WorkerID,
		ExecutorID:       row.ExecutorID,
		Status:           row.Status,
		ClaimEpoch:       claimEpoch,
		ClaimToken:       row.ClaimToken,
		LeaseExpiresAt:   db.TimeFromPg(row.LeaseExpiresAt),
		LastHeartbeatAt:  db.TimeFromPg(row.LastHeartbeatAt),
		InputManifestID:  uuidString(row.InputManifestID),
		ParkCheckpointID: uuidString(row.ParkCheckpointID),
		FailureClass:     row.FailureClass,
		TerminalReason:   row.TerminalReason,
		StartedAt:        db.TimeFromPg(row.StartedAt),
		FinishedAt:       db.TimeFromPg(row.FinishedAt),
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		UpdatedAt:        db.TimeFromPg(row.UpdatedAt),
	}
}

func toWorkerLease(row sqlc.OrchestrationWorker) WorkerLease {
	return WorkerLease{
		ID:              row.ID,
		ExecutorID:      row.ExecutorID,
		DisplayName:     row.DisplayName,
		Capabilities:    decodeJSONObject(row.Capabilities),
		Status:          row.Status,
		LeaseToken:      row.LeaseToken,
		LastHeartbeatAt: db.TimeFromPg(row.LastHeartbeatAt),
		LeaseExpiresAt:  db.TimeFromPg(row.LeaseExpiresAt),
		CreatedAt:       db.TimeFromPg(row.CreatedAt),
		UpdatedAt:       db.TimeFromPg(row.UpdatedAt),
	}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}

func normalizeWorkerProfiles(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	profiles := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, candidate := range raw {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		profiles = append(profiles, trimmed)
	}
	if len(profiles) == 0 {
		return nil
	}
	return profiles
}

func workerCapabilities(workerProfiles []string) map[string]any {
	return profileCapabilities("worker_profiles", workerProfiles)
}

func profileCapabilities(capabilityKey string, profiles []string) map[string]any {
	if len(profiles) == 0 {
		return map[string]any{}
	}
	items := make([]any, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, profile)
	}
	return map[string]any{
		capabilityKey: items,
	}
}

type plannedChildTask struct {
	Alias              string
	Kind               string
	Goal               string
	Inputs             map[string]any
	DependsOnAliases   []string
	WorkerProfile      string
	Priority           int
	RetryPolicy        map[string]any
	VerificationPolicy map[string]any
	BlackboardScope    string
}

func (s *Service) expandChildTasksFromPlans(
	ctx context.Context,
	qtx *sqlc.Queries,
	runRow sqlc.OrchestrationRun,
	parentTask sqlc.OrchestrationTask,
	attemptID pgtype.UUID,
	childPlans []plannedChildTask,
	readyReason string,
) (sqlc.OrchestrationEvent, error) {
	var lastEvent sqlc.OrchestrationEvent
	type createdChildTask struct {
		plan plannedChildTask
		row  sqlc.OrchestrationTask
	}
	createdTasks := make([]createdChildTask, 0, len(childPlans))
	aliasToTaskID := make(map[string]pgtype.UUID, len(childPlans))
	for _, plan := range childPlans {
		childTaskID, childTaskUUID, err := newPGUUID()
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
		blackboardScope := plan.BlackboardScope
		if blackboardScope == "" {
			blackboardScope = fmt.Sprintf("run.task.%s", childTaskID)
		}
		childTask, err := qtx.CreateOrchestrationTask(ctx, sqlc.CreateOrchestrationTaskParams{
			ID:                   childTaskUUID,
			RunID:                runRow.ID,
			DecomposedFromTaskID: parentTask.ID,
			Kind:                 plan.Kind,
			Goal:                 plan.Goal,
			Inputs:               marshalObject(plan.Inputs),
			PlannerEpoch:         runRow.PlannerEpoch,
			WorkerProfile:        plan.WorkerProfile,
			Priority:             clampInt32(plan.Priority),
			RetryPolicy:          marshalObject(plan.RetryPolicy),
			VerificationPolicy:   marshalObject(plan.VerificationPolicy),
			Status:               TaskStatusCreated,
			StatusVersion:        1,
			WaitingScope:         "",
			BlockedReason:        "",
			TerminalReason:       "",
			BlackboardScope:      blackboardScope,
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("create child task from structured output: %w", err)
		}
		createdEvent, err := s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           childTask.ID,
			AttemptID:        attemptID,
			AggregateType:    "task",
			AggregateID:      childTask.ID,
			AggregateVersion: childTask.StatusVersion,
			Type:             "run.event.task.created",
			Payload: map[string]any{
				"task_id":                 childTask.ID.String(),
				"run_id":                  runRow.ID.String(),
				"decomposed_from_task_id": parentTask.ID.String(),
				"previous_status":         "",
				"new_status":              childTask.Status,
				"planner_epoch":           childTask.PlannerEpoch,
				"worker_profile":          childTask.WorkerProfile,
				"blackboard_scope":        childTask.BlackboardScope,
				"depends_on":              plan.DependsOnAliases,
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
		if !lastEvent.ID.Valid {
			lastEvent = createdEvent
		}
		createdTasks = append(createdTasks, createdChildTask{plan: plan, row: childTask})
		if plan.Alias != "" {
			aliasToTaskID[plan.Alias] = childTask.ID
		}
	}
	for _, created := range createdTasks {
		if len(created.plan.DependsOnAliases) == 0 {
			continue
		}
		for _, depAlias := range created.plan.DependsOnAliases {
			predecessorID, ok := aliasToTaskID[depAlias]
			if !ok {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("%w: unknown child task dependency alias %q", ErrPlanningIntentInvalid, depAlias)
			}
			_, depUUID, err := newPGUUID()
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
			if _, err := qtx.CreateOrchestrationTaskDependency(ctx, sqlc.CreateOrchestrationTaskDependencyParams{
				ID:                       depUUID,
				RunID:                    runRow.ID,
				PredecessorTaskID:        predecessorID,
				SuccessorTaskID:          created.row.ID,
				PlannerEpoch:             runRow.PlannerEpoch,
				SupersededByPlannerEpoch: pgtype.Int8{},
			}); err != nil {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("create child task dependency: %w", err)
			}
		}
	}
	for _, created := range createdTasks {
		if len(created.plan.DependsOnAliases) > 0 {
			continue
		}
		childTask := created.row
		readyTask, err := qtx.MarkOrchestrationTaskReadyFromCheckpoint(ctx, childTask.ID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("mark child task ready from structured output: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           readyTask.ID,
			AttemptID:        attemptID,
			AggregateType:    "task",
			AggregateID:      readyTask.ID,
			AggregateVersion: readyTask.StatusVersion,
			Type:             "run.event.task.ready",
			Payload: map[string]any{
				"task_id":                 readyTask.ID.String(),
				"previous_status":         childTask.Status,
				"new_status":              readyTask.Status,
				"ready_reason":            strings.TrimSpace(readyReason),
				"decomposed_from_task_id": parentTask.ID.String(),
				"depends_on":              created.plan.DependsOnAliases,
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}
	return lastEvent, nil
}

func encodePlannedChildTasks(childPlans []plannedChildTask) []any {
	if len(childPlans) == 0 {
		return nil
	}
	items := make([]any, 0, len(childPlans))
	for _, plan := range childPlans {
		item := map[string]any{
			"alias":               plan.Alias,
			"kind":                plan.Kind,
			"goal":                plan.Goal,
			"inputs":              normalizeObject(plan.Inputs),
			"depends_on":          plan.DependsOnAliases,
			"worker_profile":      plan.WorkerProfile,
			"priority":            plan.Priority,
			"retry_policy":        normalizeObject(plan.RetryPolicy),
			"verification_policy": normalizeObject(plan.VerificationPolicy),
			"blackboard_scope":    plan.BlackboardScope,
		}
		items = append(items, item)
	}
	return items
}

func decodeReplanChildTasks(payload map[string]any) []plannedChildTask {
	if len(payload) == 0 {
		return nil
	}
	if replacementPlan, ok := payload["replacement_plan"].(map[string]any); ok {
		return decodePlannedChildTasks(replacementPlan)
	}
	return nil
}

func buildTaskSubtreeSet(tasks []sqlc.OrchestrationTask, rootTaskID pgtype.UUID) map[string]struct{} {
	childrenByParent := make(map[string][]pgtype.UUID)
	for _, task := range tasks {
		if !task.DecomposedFromTaskID.Valid {
			continue
		}
		parentID := task.DecomposedFromTaskID.String()
		childrenByParent[parentID] = append(childrenByParent[parentID], task.ID)
	}
	rootID := rootTaskID.String()
	subtree := make(map[string]struct{})
	queue := []string{rootID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := subtree[current]; seen {
			continue
		}
		subtree[current] = struct{}{}
		for _, childID := range childrenByParent[current] {
			queue = append(queue, childID.String())
		}
	}
	return subtree
}

func validatePlannedChildTasks(childPlans []plannedChildTask) error {
	if len(childPlans) == 0 {
		return fmt.Errorf("%w: planned child tasks are required", ErrPlanningIntentInvalid)
	}
	aliases := make(map[string]struct{}, len(childPlans))
	for _, plan := range childPlans {
		alias := strings.TrimSpace(plan.Alias)
		if alias == "" {
			continue
		}
		if _, exists := aliases[alias]; exists {
			return fmt.Errorf("%w: duplicate child task alias %q", ErrPlanningIntentInvalid, alias)
		}
		aliases[alias] = struct{}{}
	}
	for _, plan := range childPlans {
		for _, depAlias := range plan.DependsOnAliases {
			trimmedDepAlias := strings.TrimSpace(depAlias)
			if trimmedDepAlias == "" {
				continue
			}
			if trimmedDepAlias == strings.TrimSpace(plan.Alias) {
				return fmt.Errorf("%w: child task %q cannot depend on itself", ErrPlanningIntentInvalid, plan.Alias)
			}
			if _, exists := aliases[trimmedDepAlias]; !exists {
				return fmt.Errorf("%w: unknown child task dependency alias %q", ErrPlanningIntentInvalid, trimmedDepAlias)
			}
		}
	}
	return nil
}

func isActiveExecutionTaskStatus(status string) bool {
	switch status {
	case TaskStatusDispatching, TaskStatusRunning, TaskStatusVerifying:
		return true
	default:
		return false
	}
}

func validateReplanSubtreeIsQuiescent(
	ctx context.Context,
	qtx *sqlc.Queries,
	runID pgtype.UUID,
	subtreeTaskIDs map[string]struct{},
	allTasks []sqlc.OrchestrationTask,
	allDependencies []sqlc.OrchestrationTaskDependency,
) error {
	for _, task := range allTasks {
		if _, ok := subtreeTaskIDs[task.ID.String()]; !ok {
			continue
		}
		if isActiveExecutionTaskStatus(task.Status) {
			return fmt.Errorf("%w: task %s is still active with status %s", ErrPlanningIntentInvalid, task.ID.String(), task.Status)
		}
	}
	attempts, err := qtx.ListCurrentOrchestrationTaskAttemptsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("list current task attempts for replan: %w", err)
	}
	for _, attempt := range attempts {
		if _, ok := subtreeTaskIDs[attempt.TaskID.String()]; !ok {
			continue
		}
		if !isTerminalAttemptStatus(attempt.Status) {
			return fmt.Errorf("%w: task %s has active attempt %s with status %s", ErrPlanningIntentInvalid, attempt.TaskID.String(), attempt.ID.String(), attempt.Status)
		}
	}
	for _, dep := range allDependencies {
		if dep.SupersededByPlannerEpoch.Valid {
			continue
		}
		_, predecessorInSubtree := subtreeTaskIDs[dep.PredecessorTaskID.String()]
		_, successorInSubtree := subtreeTaskIDs[dep.SuccessorTaskID.String()]
		if predecessorInSubtree == successorInSubtree {
			continue
		}
		return fmt.Errorf(
			"%w: replan source subtree has active boundary dependency predecessor=%s successor=%s",
			ErrPlanningIntentInvalid,
			dep.PredecessorTaskID.String(),
			dep.SuccessorTaskID.String(),
		)
	}
	return nil
}

func (s *Service) releaseRunBarrierAfterCheckpointClosure(
	ctx context.Context,
	qtx *sqlc.Queries,
	runRow sqlc.OrchestrationRun,
	closedCheckpoint sqlc.OrchestrationHumanCheckpoint,
	attemptID pgtype.UUID,
	reason string,
) (sqlc.OrchestrationEvent, error) {
	var lastEvent sqlc.OrchestrationEvent
	if !closedCheckpoint.BlocksRun || runRow.LifecycleStatus != LifecycleStatusWaitingHuman {
		return lastEvent, nil
	}
	openBarrier, hasOpenBarrier, err := findOpenRunBlockingCheckpoint(ctx, qtx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("find open run blocking checkpoint: %w", err)
	}
	verifications, err := qtx.ListCurrentOrchestrationTaskVerificationsByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("list task verifications for run barrier release: %w", err)
	}
	siblingTasks, err := qtx.ListCurrentOrchestrationTasksByRun(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("list sibling tasks for run barrier release: %w", err)
	}
	for _, sibling := range siblingTasks {
		if !taskWaitingOnRunBarrier(sibling, closedCheckpoint.ID) || sibling.SupersededByPlannerEpoch.Valid {
			continue
		}
		lockedSibling, err := qtx.GetOrchestrationTaskByIDForUpdate(ctx, sibling.ID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("lock sibling task for run barrier release: %w", err)
		}
		if !taskWaitingOnRunBarrier(lockedSibling, closedCheckpoint.ID) || lockedSibling.SupersededByPlannerEpoch.Valid {
			continue
		}
		if hasOpenBarrier {
			waitingTask, err := qtx.MarkOrchestrationTaskWaitingHuman(ctx, sqlc.MarkOrchestrationTaskWaitingHumanParams{
				ID:                  lockedSibling.ID,
				WaitingCheckpointID: openBarrier.ID,
				WaitingScope:        "run",
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("rebind task to open run checkpoint: %w", err)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
				TaskID:           waitingTask.ID,
				CheckpointID:     openBarrier.ID,
				AttemptID:        attemptID,
				AggregateType:    "task",
				AggregateID:      waitingTask.ID,
				AggregateVersion: waitingTask.StatusVersion,
				Type:             "run.event.task.waiting_human",
				Payload: map[string]any{
					"task_id":                        waitingTask.ID.String(),
					"previous_status":                lockedSibling.Status,
					"new_status":                     waitingTask.Status,
					"waiting_scope":                  waitingTask.WaitingScope,
					"previous_waiting_scope":         lockedSibling.WaitingScope,
					"previous_waiting_checkpoint_id": closedCheckpoint.ID.String(),
					"waiting_checkpoint_id":          openBarrier.ID.String(),
					"waiting_reason":                 strings.TrimSpace(reason),
				},
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
			continue
		}
		if restoredVerifying, verificationRow := findRestorableRunBarrierVerification(verifications, lockedSibling.ID); restoredVerifying {
			verifyingTask, err := qtx.MarkOrchestrationTaskVerifying(ctx, sqlc.MarkOrchestrationTaskVerifyingParams{
				ID:             lockedSibling.ID,
				LatestResultID: verificationRow.ResultID,
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, fmt.Errorf("mark sibling task verifying from run barrier: %w", err)
			}
			lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
				TaskID:           verifyingTask.ID,
				CheckpointID:     closedCheckpoint.ID,
				AttemptID:        attemptID,
				AggregateType:    "task",
				AggregateID:      verifyingTask.ID,
				AggregateVersion: verifyingTask.StatusVersion,
				Type:             "run.event.task.verifying",
				Payload: map[string]any{
					"task_id":           verifyingTask.ID.String(),
					"previous_status":   lockedSibling.Status,
					"new_status":        verifyingTask.Status,
					"latest_result_id":  verificationRow.ResultID.String(),
					"verification_id":   verificationRow.ID.String(),
					"completion_reason": strings.TrimSpace(reason),
					"waiting_scope":     lockedSibling.WaitingScope,
					"checkpoint_id":     closedCheckpoint.ID.String(),
				},
			})
			if err != nil {
				return sqlc.OrchestrationEvent{}, err
			}
			continue
		}
		readySibling, err := qtx.MarkOrchestrationTaskReadyFromCheckpoint(ctx, lockedSibling.ID)
		if err != nil {
			return sqlc.OrchestrationEvent{}, fmt.Errorf("mark sibling task ready from run barrier: %w", err)
		}
		lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
			TaskID:           readySibling.ID,
			CheckpointID:     closedCheckpoint.ID,
			AttemptID:        attemptID,
			AggregateType:    "task",
			AggregateID:      readySibling.ID,
			AggregateVersion: readySibling.StatusVersion,
			Type:             "run.event.task.ready",
			Payload: map[string]any{
				"task_id":         readySibling.ID.String(),
				"previous_status": lockedSibling.Status,
				"new_status":      readySibling.Status,
				"ready_reason":    strings.TrimSpace(reason),
				"waiting_scope":   lockedSibling.WaitingScope,
				"checkpoint_id":   closedCheckpoint.ID.String(),
			},
		})
		if err != nil {
			return sqlc.OrchestrationEvent{}, err
		}
	}
	if hasOpenBarrier {
		return lastEvent, nil
	}
	runningRun, err := qtx.MarkOrchestrationRunRunning(ctx, runRow.ID)
	if err != nil {
		return sqlc.OrchestrationEvent{}, fmt.Errorf("mark run running: %w", err)
	}
	lastEvent, err = s.appendEvent(ctx, qtx, runRow.ID, eventSpec{
		CheckpointID:     closedCheckpoint.ID,
		AttemptID:        attemptID,
		AggregateType:    "run",
		AggregateID:      runningRun.ID,
		AggregateVersion: runningRun.StatusVersion,
		Type:             "run.event.running",
		Payload: map[string]any{
			"run_id":          runningRun.ID.String(),
			"previous_status": runRow.LifecycleStatus,
			"new_status":      runningRun.LifecycleStatus,
			"entry_reason":    strings.TrimSpace(reason),
			"checkpoint_id":   closedCheckpoint.ID.String(),
		},
	})
	if err != nil {
		return sqlc.OrchestrationEvent{}, err
	}
	return lastEvent, nil
}

func decodePlannedChildTasks(structuredOutput map[string]any) []plannedChildTask {
	if len(structuredOutput) == 0 {
		return nil
	}
	rawItems, ok := structuredOutput["child_tasks"].([]any)
	if !ok || len(rawItems) == 0 {
		return nil
	}
	plans := make([]plannedChildTask, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		goal := strings.TrimSpace(stringValue(item["goal"]))
		if goal == "" {
			continue
		}
		alias := strings.TrimSpace(stringValue(item["alias"]))
		if alias == "" {
			alias = strings.TrimSpace(stringValue(item["id"]))
		}
		kind := strings.TrimSpace(stringValue(item["kind"]))
		if kind == "" {
			kind = "child"
		}
		workerProfile := strings.TrimSpace(stringValue(item["worker_profile"]))
		if workerProfile == "" {
			workerProfile = DefaultRootWorkerProfile
		}
		plan := plannedChildTask{
			Alias:              alias,
			Kind:               kind,
			Goal:               goal,
			Inputs:             mapValue(item["inputs"]),
			DependsOnAliases:   stringSliceValue(firstNonNil(item["depends_on"], item["depends_on_aliases"], item["depends_on_task_ids"])),
			WorkerProfile:      workerProfile,
			Priority:           intValue(item["priority"]),
			RetryPolicy:        mapValue(item["retry_policy"]),
			VerificationPolicy: mapValue(item["verification_policy"]),
			BlackboardScope:    strings.TrimSpace(stringValue(item["blackboard_scope"])),
		}
		plans = append(plans, plan)
	}
	return plans
}

func findRestorableRunBarrierVerification(verifications []sqlc.OrchestrationTaskVerification, taskID pgtype.UUID) (bool, sqlc.OrchestrationTaskVerification) {
	for _, verification := range verifications {
		if verification.TaskID == taskID && !isTerminalVerificationStatus(verification.Status) {
			return true, verification
		}
	}
	return false, sqlc.OrchestrationTaskVerification{}
}

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

func mapValue(raw any) map[string]any {
	value, _ := raw.(map[string]any)
	return normalizeObject(value)
}

func intValue(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func stringSliceValue(raw any) []string {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	values := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.TrimSpace(stringValue(item))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func dependencyPredecessorIDs(deps []sqlc.OrchestrationTaskDependency) []string {
	values := make([]string, 0, len(deps))
	for _, dep := range deps {
		values = append(values, dep.PredecessorTaskID.String())
	}
	return values
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func clampInt32(value int) int32 {
	if value > int(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	const minInt32 = -1 << 31
	if value < minInt32 {
		return minInt32
	}
	return int32(value)
}
