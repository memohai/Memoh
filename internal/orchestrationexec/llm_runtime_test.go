package orchestrationexec

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/orchestration"
)

func TestDecodeJSONObjectTextStripsCodeFence(t *testing.T) {
	payload, err := decodeJSONObjectText("```json\n{\"status\":\"completed\",\"summary\":\"ok\"}\n```")
	if err != nil {
		t.Fatalf("decodeJSONObjectText error = %v", err)
	}
	if got := payload["status"]; got != "completed" {
		t.Fatalf("status = %v, want completed", got)
	}
}

func TestDecodeStartRunPlannerPayloadRequiresChildTasks(t *testing.T) {
	_, err := decodeStartRunPlannerPayload(map[string]any{
		"summary": "single task",
	})
	if err == nil {
		t.Fatal("decodeStartRunPlannerPayload() error = nil, want missing child_tasks error")
	}
}

func TestDecodeStartRunPlannerPayloadRejectsUnknownFields(t *testing.T) {
	_, err := decodeStartRunPlannerPayload(map[string]any{
		"summary":     "split",
		"child_tasks": []any{map[string]any{"goal": "step a", "depends_on_aliases": []any{}}},
	})
	if err == nil {
		t.Fatal("decodeStartRunPlannerPayload() error = nil, want unknown field error")
	}
}

func TestDecodeStartRunPlannerPayloadValidatesChildTasks(t *testing.T) {
	plan, err := decodeStartRunPlannerPayload(map[string]any{
		"summary": "split",
		"child_tasks": []any{
			map[string]any{
				"alias":               "a",
				"kind":                "step",
				"goal":                "step a",
				"inputs":              map[string]any{"x": "1"},
				"depends_on":          []any{},
				"worker_profile":      "llm.default",
				"priority":            float64(2),
				"retry_policy":        map[string]any{},
				"verification_policy": map[string]any{"mode": "builtin.basic"},
				"blackboard_scope":    "run.a",
			},
		},
	})
	if err != nil {
		t.Fatalf("decodeStartRunPlannerPayload() error = %v", err)
	}
	if plan.Summary != "split" || len(plan.ChildTasks) != 1 {
		t.Fatalf("plan = %#v, want summary and one child", plan)
	}
	child := plan.ChildTasks[0]
	if child.Alias != "a" || child.Goal != "step a" || child.Priority != 2 {
		t.Fatalf("child = %#v, want normalized task fields", child)
	}
	if child.VerificationPolicy["mode"] != "builtin.basic" {
		t.Fatalf("verification policy = %#v, want preserved policy", child.VerificationPolicy)
	}
}

func TestDecodeStartRunPlannerPayloadRejectsFractionalPriority(t *testing.T) {
	_, err := decodeStartRunPlannerPayload(map[string]any{
		"summary": "split",
		"child_tasks": []any{
			map[string]any{
				"goal":     "step a",
				"priority": float64(1.5),
			},
		},
	})
	if err == nil {
		t.Fatal("decodeStartRunPlannerPayload() error = nil, want fractional priority error")
	}
}

func TestDecodeReplanPlannerPayloadUsesStrictChildTaskSchema(t *testing.T) {
	plan, err := decodeReplanPlannerPayload(map[string]any{
		"summary": "replace failed task",
		"child_tasks": []any{
			map[string]any{
				"alias":          "replacement",
				"goal":           "repair and rerun the failed step",
				"worker_profile": "llm.default",
				"depends_on":     []any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("decodeReplanPlannerPayload() error = %v", err)
	}
	if plan.Summary != "replace failed task" || len(plan.ChildTasks) != 1 {
		t.Fatalf("plan = %#v, want summary and one child", plan)
	}
	if plan.ChildTasks[0].Alias != "replacement" {
		t.Fatalf("child alias = %q, want replacement", plan.ChildTasks[0].Alias)
	}
}

func TestDecodeReplanPlannerPayloadRejectsLegacyAliases(t *testing.T) {
	tests := []struct {
		name string
		task map[string]any
	}{
		{
			name: "id",
			task: map[string]any{
				"id":   "legacy",
				"goal": "repair and rerun the failed step",
			},
		},
		{
			name: "depends_on_aliases",
			task: map[string]any{
				"alias":              "replacement",
				"goal":               "repair and rerun the failed step",
				"depends_on_aliases": []any{"legacy"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeReplanPlannerPayload(map[string]any{
				"summary":     "replace failed task",
				"child_tasks": []any{tt.task},
			})
			if err == nil {
				t.Fatal("decodeReplanPlannerPayload() error = nil, want legacy field error")
			}
		})
	}
}

func TestDecodeReplanPlannerPayloadRejectsEmptyReplacement(t *testing.T) {
	_, err := decodeReplanPlannerPayload(map[string]any{
		"summary":     "no safe replacement",
		"child_tasks": []any{},
	})
	if err == nil {
		t.Fatal("decodeReplanPlannerPayload() error = nil, want empty replacement error")
	}
}

func TestBuildReplanPlannerPromptIncludesFailureContext(t *testing.T) {
	prompt := buildReplanPlannerPrompt(orchestration.ReplanPlanningInput{
		Run: orchestration.Run{
			ID:             "run-1",
			Goal:           "ship the report",
			PlannerEpoch:   2,
			SourceMetadata: map[string]any{"private_note": "do not send to replanner"},
		},
		SourceTask: orchestration.Task{
			ID:   "task-1",
			Goal: "write the report",
		},
		SourceAttempt: &orchestration.TaskAttempt{
			ID:              "attempt-1",
			Status:          orchestration.TaskAttemptStatusFailed,
			WorkerID:        "internal-worker-id",
			ExecutorID:      "internal-executor-id",
			ClaimToken:      "secret-claim-token",
			InputManifestID: "internal-manifest-id",
		},
		SourceResult: &orchestration.TaskResult{
			ID:      "result-1",
			Status:  orchestration.TaskAttemptStatusFailed,
			Summary: "missing required artifact",
		},
		SubtreeTasks: []orchestration.Task{
			{ID: "task-1", Goal: "write the report"},
		},
		Reason:       "verification rejected output",
		InjectedHint: map[string]any{"constraint": "keep it concise"},
	})
	for _, expected := range []string{
		"ship the report",
		"write the report",
		"missing required artifact",
		"verification rejected output",
		"keep it concise",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("replan prompt missing %q: %s", expected, prompt)
		}
	}
	if strings.Contains(prompt, "secret-claim-token") || strings.Contains(prompt, "claim_token") {
		t.Fatalf("replan prompt leaked claim token fields: %s", prompt)
	}
	for _, unexpected := range []string{"do not send to replanner", "private_note", "internal-worker-id", "internal-executor-id", "internal-manifest-id"} {
		if strings.Contains(prompt, unexpected) {
			t.Fatalf("replan prompt leaked %q: %s", unexpected, prompt)
		}
	}
}

func TestDecodeAttemptCompletionPayloadPromotesTopLevelChildTasks(t *testing.T) {
	completion, err := decodeAttemptCompletionPayload(
		orchestration.TaskAttempt{ID: "attempt-1", ClaimToken: "claim-1"},
		sqlc.OrchestrationTask{Goal: "compute fib"},
		map[string]any{
			"status":         "completed",
			"summary":        "needs decomposition",
			"request_replan": true,
			"child_tasks": []any{
				map[string]any{"goal": "step a"},
			},
		},
	)
	if err != nil {
		t.Fatalf("decodeAttemptCompletionPayload error = %v", err)
	}
	if !completion.RequestReplan {
		t.Fatal("RequestReplan = false, want true")
	}
	childTasks, ok := completion.StructuredOutput["child_tasks"].([]any)
	if !ok || len(childTasks) != 1 {
		t.Fatalf("structured_output.child_tasks = %#v, want 1 item", completion.StructuredOutput["child_tasks"])
	}
}

func TestDecodeVerificationCompletionPayloadDefaultsRejectReason(t *testing.T) {
	completion, err := decodeVerificationCompletionPayload(
		orchestration.TaskVerification{ID: "verification-1", ClaimToken: "claim-1"},
		sqlc.OrchestrationTask{Goal: "verify fib"},
		sqlc.OrchestrationTaskResult{Summary: "worker said fib=1346269"},
		map[string]any{
			"status":  "failed",
			"verdict": "rejected",
			"summary": "result is inconsistent",
		},
	)
	if err != nil {
		t.Fatalf("decodeVerificationCompletionPayload error = %v", err)
	}
	if completion.TerminalReason != "result is inconsistent" {
		t.Fatalf("TerminalReason = %q, want summary fallback", completion.TerminalReason)
	}
}
