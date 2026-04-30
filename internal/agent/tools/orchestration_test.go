package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/orchestration"
)

type fakeOrchestrationToolService struct {
	startRun           func(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error)
	cancelRun          func(context.Context, orchestration.ControlIdentity, string, orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error)
	getRunSnapshot     func(context.Context, orchestration.ControlIdentity, string) (*orchestration.RunSnapshot, error)
	listRunTasks       func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error)
	listRunCheckpoints func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error)
	listRunEvents      func(context.Context, orchestration.ControlIdentity, string, orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error)
	resolveCheckpoint  func(context.Context, orchestration.ControlIdentity, string, orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error)
}

func (f fakeOrchestrationToolService) StartRun(ctx context.Context, caller orchestration.ControlIdentity, req orchestration.StartRunRequest) (orchestration.RunHandle, error) {
	return f.startRun(ctx, caller, req)
}

func (f fakeOrchestrationToolService) CancelRun(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error) {
	return f.cancelRun(ctx, caller, runID, req)
}

func (f fakeOrchestrationToolService) GetRunSnapshot(ctx context.Context, caller orchestration.ControlIdentity, runID string) (*orchestration.RunSnapshot, error) {
	return f.getRunSnapshot(ctx, caller, runID)
}

func (f fakeOrchestrationToolService) ListRunTasks(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error) {
	return f.listRunTasks(ctx, caller, runID, req)
}

func (f fakeOrchestrationToolService) ListRunCheckpoints(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error) {
	return f.listRunCheckpoints(ctx, caller, runID, req)
}

func (f fakeOrchestrationToolService) ListRunEvents(ctx context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error) {
	return f.listRunEvents(ctx, caller, runID, req)
}

func (f fakeOrchestrationToolService) ResolveCheckpoint(ctx context.Context, caller orchestration.ControlIdentity, checkpointID string, req orchestration.CheckpointResolution) (*orchestration.ResolveCheckpointResult, error) {
	return f.resolveCheckpoint(ctx, caller, checkpointID, req)
}

type fakeOrchestrationBotReader struct {
	get func(context.Context, string) (bots.Bot, error)
}

func (f fakeOrchestrationBotReader) Get(ctx context.Context, botID string) (bots.Bot, error) {
	return f.get(ctx, botID)
}

func TestOrchestrationToolStartUsesBotOwnerIdentity(t *testing.T) {
	provider := NewOrchestrationProvider(nil, fakeOrchestrationToolService{
		startRun: func(_ context.Context, caller orchestration.ControlIdentity, req orchestration.StartRunRequest) (orchestration.RunHandle, error) {
			if caller != (orchestration.ControlIdentity{TenantID: "owner-1", Subject: "owner-1"}) {
				t.Fatalf("caller = %+v", caller)
			}
			if req.BotID != "bot-1" {
				t.Fatalf("bot_id = %q", req.BotID)
			}
			if req.Goal != "ship orchestration tool" {
				t.Fatalf("goal = %q", req.Goal)
			}
			if req.IdempotencyKey != "run-1" {
				t.Fatalf("idempotency_key = %q", req.IdempotencyKey)
			}
			if got := req.SourceMetadata["bot_id"]; got != "bot-1" {
				t.Fatalf("source_metadata.bot_id = %#v, want %q", got, "bot-1")
			}
			return orchestration.RunHandle{RunID: "run-1", RootTaskID: "task-1", SnapshotSeq: 7}, nil
		},
	}, fakeOrchestrationBotReader{
		get: func(_ context.Context, botID string) (bots.Bot, error) {
			if botID != "bot-1" {
				t.Fatalf("botID = %q", botID)
			}
			return bots.Bot{ID: botID, OwnerUserID: "owner-1"}, nil
		},
	})

	tools, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	result, err := tools[0].Execute(nil, map[string]any{
		"action":          "start",
		"goal":            "ship orchestration tool",
		"idempotency_key": "run-1",
	})
	if err != nil {
		t.Fatalf("Execute(start) error = %v", err)
	}
	payload := result.(map[string]any)
	if payload["run_id"] != "run-1" {
		t.Fatalf("run_id = %#v, want %q", payload["run_id"], "run-1")
	}
}

func TestOrchestrationToolStatusSummarizesRun(t *testing.T) {
	provider := NewOrchestrationProvider(nil, fakeOrchestrationToolService{
		getRunSnapshot: func(_ context.Context, _ orchestration.ControlIdentity, runID string) (*orchestration.RunSnapshot, error) {
			return &orchestration.RunSnapshot{
				Run: orchestration.Run{
					ID:              runID,
					Goal:            "inspect run",
					LifecycleStatus: orchestration.LifecycleStatusWaitingHuman,
					PlanningStatus:  orchestration.PlanningStatusIdle,
				},
				SnapshotSeq: 12,
			}, nil
		},
		listRunTasks: func(_ context.Context, _ orchestration.ControlIdentity, _ string, req orchestration.ListRunTasksRequest) (*orchestration.TaskPage, error) {
			if req.AsOfSeq != 12 {
				t.Fatalf("as_of_seq = %d, want %d", req.AsOfSeq, 12)
			}
			return &orchestration.TaskPage{Items: []orchestration.Task{
				{ID: "task-a", Goal: "wait for approval", Status: orchestration.TaskStatusWaitingHuman},
				{ID: "task-b", Goal: "done", Status: orchestration.TaskStatusCompleted},
				{ID: "task-c", Goal: "blocked by dependency", Status: orchestration.TaskStatusBlocked},
				{ID: "task-d", Goal: "cancelled branch", Status: orchestration.TaskStatusCancelled},
				{ID: "task-e", Goal: "actual failure", Status: orchestration.TaskStatusFailed},
			}}, nil
		},
		listRunCheckpoints: func(_ context.Context, _ orchestration.ControlIdentity, _ string, req orchestration.ListRunCheckpointsRequest) (*orchestration.HumanCheckpointPage, error) {
			if req.AsOfSeq != 12 {
				t.Fatalf("checkpoint as_of_seq = %d, want %d", req.AsOfSeq, 12)
			}
			return &orchestration.HumanCheckpointPage{Items: []orchestration.HumanCheckpoint{
				{
					ID:        "cp-1",
					TaskID:    "task-a",
					Question:  "approve?",
					Status:    orchestration.CheckpointStatusOpen,
					BlocksRun: true,
					Options:   []orchestration.CheckpointOption{{ID: "approve", Kind: "choice", Label: "Approve"}},
					DefaultAction: &orchestration.CheckpointDefaultAction{
						Mode:     orchestration.CheckpointResolutionModeSelectOption,
						OptionID: "approve",
					},
					ResumePolicy: &orchestration.CheckpointResumePolicy{ResumeMode: orchestration.CheckpointResumeModeNewAttempt},
				},
			}}, nil
		},
		listRunEvents: func(_ context.Context, _ orchestration.ControlIdentity, _ string, req orchestration.ListRunEventsRequest) (*orchestration.RunEventPage, error) {
			if req.UntilSeq != 12 {
				t.Fatalf("until_seq = %d, want %d", req.UntilSeq, 12)
			}
			return &orchestration.RunEventPage{Items: []orchestration.RunEvent{
				{Seq: 12, Type: "run.event.waiting_human", TaskID: "task-a"},
			}}, nil
		},
	}, fakeOrchestrationBotReader{
		get: func(_ context.Context, _ string) (bots.Bot, error) {
			return bots.Bot{ID: "bot-1", OwnerUserID: "owner-1"}, nil
		},
	})

	tools, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	result, err := tools[0].Execute(nil, map[string]any{
		"action": "status",
		"run_id": "run-1",
	})
	if err != nil {
		t.Fatalf("Execute(status) error = %v", err)
	}
	payload := result.(map[string]any)
	if payload["lifecycle_status"] != orchestration.LifecycleStatusWaitingHuman {
		t.Fatalf("lifecycle_status = %#v", payload["lifecycle_status"])
	}
	if payload["completed_tasks"] != 1 {
		t.Fatalf("completed_tasks = %#v, want %d", payload["completed_tasks"], 1)
	}
	if payload["failed_tasks"] != 1 {
		t.Fatalf("failed_tasks = %#v, want %d", payload["failed_tasks"], 1)
	}
	openCheckpoints, ok := payload["open_checkpoints"].([]map[string]any)
	if !ok {
		t.Fatalf("open_checkpoints type = %T", payload["open_checkpoints"])
	}
	if len(openCheckpoints) != 1 {
		t.Fatalf("len(open_checkpoints) = %d, want 1", len(openCheckpoints))
	}
	if openCheckpoints[0]["default_action"] == nil {
		t.Fatal("default_action = nil, want populated checkpoint resolution hint")
	}
	if openCheckpoints[0]["resume_policy"] == nil {
		t.Fatal("resume_policy = nil, want populated resume policy")
	}
	options, ok := openCheckpoints[0]["options"].([]orchestration.CheckpointOption)
	if !ok || len(options) != 1 || options[0].ID != "approve" {
		t.Fatalf("checkpoint options = %#v", openCheckpoints[0]["options"])
	}
}

func TestOrchestrationToolHiddenForSubagent(t *testing.T) {
	provider := NewOrchestrationProvider(nil, fakeOrchestrationToolService{}, fakeOrchestrationBotReader{
		get: func(context.Context, string) (bots.Bot, error) {
			return bots.Bot{}, nil
		},
	})

	tools, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1", IsSubagent: true})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(tools))
	}
}

func TestOrchestrationToolCancelForwardsRequest(t *testing.T) {
	provider := NewOrchestrationProvider(nil, fakeOrchestrationToolService{
		cancelRun: func(_ context.Context, caller orchestration.ControlIdentity, runID string, req orchestration.CancelRunRequest) (*orchestration.CancelRunResult, error) {
			if caller != (orchestration.ControlIdentity{TenantID: "owner-1", Subject: "owner-1"}) {
				t.Fatalf("caller = %+v", caller)
			}
			if runID != "run-1" {
				t.Fatalf("runID = %q", runID)
			}
			if req.IdempotencyKey != "cancel-1" {
				t.Fatalf("idempotency_key = %q", req.IdempotencyKey)
			}
			return &orchestration.CancelRunResult{
				RunID:           runID,
				LifecycleStatus: orchestration.LifecycleStatusCancelling,
				SnapshotSeq:     19,
			}, nil
		},
	}, fakeOrchestrationBotReader{
		get: func(_ context.Context, _ string) (bots.Bot, error) {
			return bots.Bot{ID: "bot-1", OwnerUserID: "owner-1"}, nil
		},
	})

	tools, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	result, err := tools[0].Execute(nil, map[string]any{
		"action":          "cancel",
		"run_id":          "run-1",
		"idempotency_key": "cancel-1",
	})
	if err != nil {
		t.Fatalf("Execute(cancel) error = %v", err)
	}
	payload := result.(map[string]any)
	if payload["lifecycle_status"] != orchestration.LifecycleStatusCancelling {
		t.Fatalf("lifecycle_status = %#v, want %q", payload["lifecycle_status"], orchestration.LifecycleStatusCancelling)
	}
	if payload["snapshot_seq"] != uint64(19) {
		t.Fatalf("snapshot_seq = %#v, want %d", payload["snapshot_seq"], 19)
	}
}

func TestOrchestrationToolRequiresOwnerLookup(t *testing.T) {
	provider := NewOrchestrationProvider(nil, fakeOrchestrationToolService{
		startRun: func(context.Context, orchestration.ControlIdentity, orchestration.StartRunRequest) (orchestration.RunHandle, error) {
			return orchestration.RunHandle{}, nil
		},
	}, fakeOrchestrationBotReader{
		get: func(context.Context, string) (bots.Bot, error) {
			return bots.Bot{}, errors.New("bot missing")
		},
	})

	tools, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	_, err = tools[0].Execute(nil, map[string]any{
		"action":          "start",
		"goal":            "ship orchestration tool",
		"idempotency_key": "run-1",
	})
	if err == nil {
		t.Fatal("Execute(start) error = nil, want owner lookup failure")
	}
}
