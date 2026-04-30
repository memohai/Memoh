package orchestrationexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/orchestration"
)

func (r *Runtime) PlanStartRun(ctx context.Context, input orchestration.StartRunPlanningInput) (*orchestration.StartRunPlanningResult, error) {
	if r == nil {
		return nil, errors.New("planner runtime is not configured")
	}
	botID := strings.TrimSpace(stringValue(input.Run.SourceMetadata["bot_id"]))
	if botID == "" {
		return &orchestration.StartRunPlanningResult{}, nil
	}
	cfg, _, _, err := r.buildBotRunConfig(ctx, botID, input.Run.OwnerSubject)
	if err != nil {
		return nil, err
	}
	cfg.System = startRunPlannerSystemPrompt
	cfg.Messages = []sdk.Message{sdk.UserMessage(buildStartRunPlannerPrompt(input))}
	cfg.ResponseFormat = &sdk.ResponseFormat{Type: sdk.ResponseFormatJSONObject}
	cfg.SupportsToolCall = false

	result, err := r.generateWithThinkingTrace(ctx, cfg, nil, nil)
	if err != nil {
		return nil, err
	}
	payload, err := decodeJSONObjectText(result.Text)
	if err != nil {
		return nil, fmt.Errorf("decode start run planner response: %w", err)
	}
	childTasks := decodeStartRunPlannedTasks(payload["child_tasks"])
	return &orchestration.StartRunPlanningResult{
		Summary:    strings.TrimSpace(stringValue(payload["summary"])),
		ChildTasks: childTasks,
	}, nil
}

func buildStartRunPlannerPrompt(input orchestration.StartRunPlanningInput) string {
	payload := map[string]any{
		"run": map[string]any{
			"goal":            input.Run.Goal,
			"input":           input.Run.Input,
			"output_schema":   input.Run.OutputSchema,
			"source_metadata": input.Run.SourceMetadata,
		},
		"root_task": map[string]any{
			"id":             input.RootTask.ID,
			"goal":           input.RootTask.Goal,
			"inputs":         input.RootTask.Inputs,
			"worker_profile": input.RootTask.WorkerProfile,
		},
	}
	raw, err := marshalJSONValue(payload)
	if err != nil {
		return `{}`
	}
	return string(raw)
}

func decodeStartRunPlannedTasks(raw any) []orchestration.PlannedTaskSpec {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	plans := make([]orchestration.PlannedTaskSpec, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		goal := strings.TrimSpace(stringValue(obj["goal"]))
		if goal == "" {
			continue
		}
		plans = append(plans, orchestration.PlannedTaskSpec{
			Alias:              strings.TrimSpace(stringValue(obj["alias"])),
			Kind:               strings.TrimSpace(stringValue(obj["kind"])),
			Goal:               goal,
			Inputs:             decodePlannerObject(obj["inputs"]),
			DependsOnAliases:   plannerStringSlice(firstNonNil(obj["depends_on"], obj["depends_on_aliases"], obj["depends_on_task_ids"])),
			WorkerProfile:      strings.TrimSpace(stringValue(obj["worker_profile"])),
			Priority:           plannerInt(obj["priority"]),
			RetryPolicy:        decodePlannerObject(obj["retry_policy"]),
			VerificationPolicy: decodePlannerObject(obj["verification_policy"]),
			BlackboardScope:    strings.TrimSpace(stringValue(obj["blackboard_scope"])),
		})
	}
	return plans
}

func decodePlannerObject(raw any) map[string]any {
	value, _ := raw.(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func plannerStringSlice(raw any) []string {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(stringValue(item))
		if text == "" {
			continue
		}
		values = append(values, text)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func plannerInt(raw any) int {
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

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

const startRunPlannerSystemPrompt = `You are the initial planner for a Memoh orchestration run.

Decide whether the root goal should start as a single executable task or first be decomposed into a small DAG of leaf tasks.

Rules:
- If the goal is already a single concrete task, return an empty child_tasks array.
- If the goal obviously contains multiple stages, validation gates, or parallelizable branches, decompose it.
- Only output executable leaf tasks. Do not output abstract manager/planner/meta tasks.
- Keep the graph small and useful. Prefer 2-5 child tasks unless the request clearly needs more.
- Use depends_on aliases to form an acyclic DAG.
- Default worker_profile should usually be "llm.default".
- Use verification_policy only when a child clearly needs an explicit verifier gate.
- Do not call tools.

Return JSON:
{
  "summary": string,
  "child_tasks": [
    {
      "alias": string,
      "kind": string,
      "goal": string,
      "inputs": object,
      "depends_on": [string],
      "worker_profile": string,
      "priority": integer,
      "retry_policy": object,
      "verification_policy": object,
      "blackboard_scope": string
    }
  ]
}`
