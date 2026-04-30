package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/orchestration"
)

type orchestrationService interface {
	StartRun(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error)
	CancelRun(context.Context, orchestration.ControlIdentity, string, orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error)
	GetRunSnapshot(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error)
	ListRunTasks(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error)
	ListRunCheckpoints(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error)
	ListRunEvents(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error)
	ResolveCheckpoint(context.Context, orchestration.ControlIdentity, string, orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error)
}

type orchestrationBotReader interface {
	Get(context.Context, string) (bots.Bot, error)
}

type OrchestrationProvider struct {
	service orchestrationService
	bots    orchestrationBotReader
	logger  *slog.Logger
}

func NewOrchestrationProvider(log *slog.Logger, service orchestrationService, bots orchestrationBotReader) *OrchestrationProvider {
	if log == nil {
		log = slog.Default()
	}
	return &OrchestrationProvider{
		service: service,
		bots:    bots,
		logger:  log.With(slog.String("tool", "orchestrate")),
	}
}

func (p *OrchestrationProvider) Tools(ctx context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.service == nil || p.bots == nil {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{{
		Name:        "orchestrate",
		Description: "Manage orchestration runs for the current bot. Supported actions: start, status, resolve, cancel.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"start", "status", "resolve", "cancel"},
					"description": "Operation to execute.",
				},
				"goal":            map[string]any{"type": "string", "description": "Run goal for action=start."},
				"input":           map[string]any{"type": "object", "description": "Optional structured run input for action=start."},
				"output_schema":   map[string]any{"type": "object", "description": "Optional output schema for action=start."},
				"run_id":          map[string]any{"type": "string", "description": "Run ID for action=status or action=cancel."},
				"checkpoint_id":   map[string]any{"type": "string", "description": "Checkpoint ID for action=resolve."},
				"mode":            map[string]any{"type": "string", "description": "Checkpoint resolution mode for action=resolve."},
				"option_id":       map[string]any{"type": "string", "description": "Checkpoint option ID for action=resolve."},
				"freeform_input":  map[string]any{"type": "string", "description": "Freeform checkpoint response for action=resolve."},
				"idempotency_key": map[string]any{"type": "string", "description": "Idempotency key for action=start, action=resolve, and action=cancel."},
			},
			"required": []string{"action"},
		},
		Execute: func(execCtx *sdk.ToolExecContext, input any) (any, error) {
			_ = execCtx
			return p.execute(ctx, sess, inputAsMap(input))
		},
	}}, nil
}

func (p *OrchestrationProvider) execute(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	action := strings.ToLower(StringArg(args, "action"))
	if action == "" {
		return nil, errors.New("action is required")
	}
	caller, err := p.controlIdentityForSession(ctx, session)
	if err != nil {
		return nil, err
	}
	switch action {
	case "start":
		return p.executeStart(ctx, session, caller, args)
	case "status":
		return p.executeStatus(ctx, caller, args)
	case "resolve":
		return p.executeResolve(ctx, caller, args)
	case "cancel":
		return p.executeCancel(ctx, caller, args)
	default:
		return nil, fmt.Errorf("unsupported action %q", action)
	}
}

func (p *OrchestrationProvider) executeStart(ctx context.Context, session SessionContext, caller orchestration.ControlIdentity, args map[string]any) (any, error) {
	goal := StringArg(args, "goal")
	if goal == "" {
		return nil, errors.New("goal is required for action=start")
	}
	idempotencyKey := StringArg(args, "idempotency_key")
	if idempotencyKey == "" {
		return nil, errors.New("idempotency_key is required for action=start")
	}
	handle, err := p.service.StartRun(ctx, caller, orchestration.StartRunRequest{
		Goal:           goal,
		BotID:          strings.TrimSpace(session.BotID),
		Input:          objectArg(args, "input"),
		OutputSchema:   objectArg(args, "output_schema"),
		IdempotencyKey: idempotencyKey,
		SourceMetadata: map[string]any{
			"bot_id":     strings.TrimSpace(session.BotID),
			"session_id": strings.TrimSpace(session.SessionID),
			"chat_id":    strings.TrimSpace(session.ChatID),
		},
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"run_id":         handle.RunID,
		"root_task_id":   handle.RootTaskID,
		"snapshot_seq":   handle.SnapshotSeq,
		"status_message": "orchestration run started",
	}, nil
}

func (p *OrchestrationProvider) executeStatus(ctx context.Context, caller orchestration.ControlIdentity, args map[string]any) (any, error) {
	runID := StringArg(args, "run_id")
	if runID == "" {
		return nil, errors.New("run_id is required for action=status")
	}
	snapshot, err := p.service.GetRunSnapshot(ctx, caller, runID)
	if err != nil {
		return nil, err
	}
	asOfSeq := snapshot.SnapshotSeq
	taskPage, err := p.service.ListRunTasks(ctx, caller, runID, orchestration.ListRunTasksRequest{
		Limit:   200,
		AsOfSeq: asOfSeq,
	})
	if err != nil {
		return nil, err
	}
	checkpointPage, err := p.service.ListRunCheckpoints(ctx, caller, runID, orchestration.ListRunCheckpointsRequest{
		Status:  []string{orchestration.CheckpointStatusOpen},
		Limit:   50,
		AsOfSeq: asOfSeq,
	})
	if err != nil {
		return nil, err
	}
	afterSeq := uint64(0)
	if asOfSeq > 10 {
		afterSeq = asOfSeq - 10
	}
	eventPage, err := p.service.ListRunEvents(ctx, caller, runID, orchestration.ListRunEventsRequest{
		AfterSeq: afterSeq,
		UntilSeq: asOfSeq,
		Limit:    10,
	})
	if err != nil {
		return nil, err
	}
	activeTasks := make([]map[string]any, 0)
	readyTasks := 0
	completedTasks := 0
	failedTasks := 0
	for _, task := range taskPage.Items {
		switch task.Status {
		case orchestration.TaskStatusReady:
			readyTasks++
			activeTasks = append(activeTasks, summarizeTask(task))
		case orchestration.TaskStatusCompleted:
			completedTasks++
		case orchestration.TaskStatusFailed:
			failedTasks++
		default:
			activeTasks = append(activeTasks, summarizeTask(task))
		}
	}
	openCheckpoints := make([]map[string]any, 0, len(checkpointPage.Items))
	for _, checkpoint := range checkpointPage.Items {
		openCheckpoints = append(openCheckpoints, summarizeCheckpoint(checkpoint))
	}
	latestEvents := make([]map[string]any, 0, len(eventPage.Items))
	for _, event := range eventPage.Items {
		latestEvents = append(latestEvents, map[string]any{
			"seq":           event.Seq,
			"type":          event.Type,
			"task_id":       event.TaskID,
			"attempt_id":    event.AttemptID,
			"checkpoint_id": event.CheckpointID,
			"created_at":    event.CreatedAt,
		})
	}
	return map[string]any{
		"run_id":           snapshot.Run.ID,
		"goal":             snapshot.Run.Goal,
		"lifecycle_status": snapshot.Run.LifecycleStatus,
		"planning_status":  snapshot.Run.PlanningStatus,
		"snapshot_seq":     snapshot.SnapshotSeq,
		"terminal_reason":  snapshot.Run.TerminalReason,
		"open_checkpoints": openCheckpoints,
		"active_tasks":     activeTasks,
		"ready_tasks":      readyTasks,
		"completed_tasks":  completedTasks,
		"failed_tasks":     failedTasks,
		"latest_events":    latestEvents,
		"status_message":   summarizeRunStatus(snapshot.Run, len(openCheckpoints), len(activeTasks)),
	}, nil
}

func (p *OrchestrationProvider) executeResolve(ctx context.Context, caller orchestration.ControlIdentity, args map[string]any) (any, error) {
	checkpointID := StringArg(args, "checkpoint_id")
	if checkpointID == "" {
		return nil, errors.New("checkpoint_id is required for action=resolve")
	}
	mode := StringArg(args, "mode")
	if mode == "" {
		return nil, errors.New("mode is required for action=resolve")
	}
	idempotencyKey := StringArg(args, "idempotency_key")
	if idempotencyKey == "" {
		return nil, errors.New("idempotency_key is required for action=resolve")
	}
	result, err := p.service.ResolveCheckpoint(ctx, caller, checkpointID, orchestration.CheckpointResolution{
		Mode:           mode,
		OptionID:       StringArg(args, "option_id"),
		FreeformInput:  StringArg(args, "freeform_input"),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"checkpoint_id":  checkpointID,
		"snapshot_seq":   result.SnapshotSeq,
		"status_message": "checkpoint resolved",
	}, nil
}

func (p *OrchestrationProvider) executeCancel(ctx context.Context, caller orchestration.ControlIdentity, args map[string]any) (any, error) {
	runID := StringArg(args, "run_id")
	if runID == "" {
		return nil, errors.New("run_id is required for action=cancel")
	}
	idempotencyKey := StringArg(args, "idempotency_key")
	if idempotencyKey == "" {
		return nil, errors.New("idempotency_key is required for action=cancel")
	}
	result, err := p.service.CancelRun(ctx, caller, runID, orchestration.CancelRunRequest{
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"run_id":           result.RunID,
		"lifecycle_status": result.LifecycleStatus,
		"snapshot_seq":     result.SnapshotSeq,
		"status_message":   "orchestration run cancellation requested",
	}, nil
}

func (p *OrchestrationProvider) controlIdentityForSession(ctx context.Context, session SessionContext) (orchestration.ControlIdentity, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return orchestration.ControlIdentity{}, errors.New("bot_id is required")
	}
	bot, err := p.bots.Get(ctx, botID)
	if err != nil {
		return orchestration.ControlIdentity{}, err
	}
	ownerID := strings.TrimSpace(bot.OwnerUserID)
	if ownerID == "" {
		return orchestration.ControlIdentity{}, errors.New("bot owner is required")
	}
	return orchestration.ControlIdentity{
		TenantID: ownerID,
		Subject:  ownerID,
	}, nil
}

func objectArg(arguments map[string]any, key string) map[string]any {
	if arguments == nil {
		return map[string]any{}
	}
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return map[string]any{}
	}
	value, ok := raw.(map[string]any)
	if !ok || value == nil {
		return map[string]any{}
	}
	return value
}

func summarizeTask(task orchestration.Task) map[string]any {
	return map[string]any{
		"id":                    task.ID,
		"goal":                  task.Goal,
		"status":                task.Status,
		"worker_profile":        task.WorkerProfile,
		"waiting_scope":         task.WaitingScope,
		"waiting_checkpoint_id": task.WaitingCheckpointID,
		"latest_result_id":      task.LatestResultID,
	}
}

func summarizeCheckpoint(checkpoint orchestration.HumanCheckpoint) map[string]any {
	return map[string]any{
		"id":             checkpoint.ID,
		"task_id":        checkpoint.TaskID,
		"question":       checkpoint.Question,
		"blocks_run":     checkpoint.BlocksRun,
		"status":         checkpoint.Status,
		"options":        checkpoint.Options,
		"default_action": checkpoint.DefaultAction,
		"resume_policy":  checkpoint.ResumePolicy,
		"timeout_at":     checkpoint.TimeoutAt,
	}
}

func summarizeRunStatus(run orchestration.Run, openCheckpoints int, activeTasks int) string {
	switch run.LifecycleStatus {
	case orchestration.LifecycleStatusWaitingHuman:
		return fmt.Sprintf("run is waiting for human input with %d open checkpoints", openCheckpoints)
	case orchestration.LifecycleStatusCompleted:
		return "run completed"
	case orchestration.LifecycleStatusFailed:
		if strings.TrimSpace(run.TerminalReason) != "" {
			return "run failed: " + strings.TrimSpace(run.TerminalReason)
		}
		return "run failed"
	case orchestration.LifecycleStatusCancelled:
		return "run cancelled"
	case orchestration.LifecycleStatusCancelling:
		return "run is cancelling"
	default:
		if activeTasks > 0 {
			return fmt.Sprintf("run is active with %d non-terminal tasks", activeTasks)
		}
		return "run is active"
	}
}
