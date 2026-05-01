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
	plan, err := decodeStartRunPlannerPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("decode start run planner schema: %w", err)
	}
	return plan, nil
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

func decodeStartRunPlannerPayload(payload map[string]any) (*orchestration.StartRunPlanningResult, error) {
	if payload == nil {
		return nil, errors.New("planner response must be a JSON object")
	}
	if err := rejectUnknownPlannerKeys(payload, map[string]struct{}{
		"summary":     {},
		"child_tasks": {},
	}); err != nil {
		return nil, err
	}
	summary, err := optionalPlannerString(payload, "summary")
	if err != nil {
		return nil, err
	}
	rawChildTasks, ok := payload["child_tasks"]
	if !ok {
		return nil, errors.New("child_tasks is required")
	}
	items, ok := rawChildTasks.([]any)
	if !ok {
		return nil, errors.New("child_tasks must be an array")
	}
	childTasks := make([]orchestration.PlannedTaskSpec, 0, len(items))
	for index, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("child_tasks[%d] must be an object", index)
		}
		task, err := decodeStartRunPlannerTask(obj, index)
		if err != nil {
			return nil, err
		}
		childTasks = append(childTasks, task)
	}
	return &orchestration.StartRunPlanningResult{
		Summary:    summary,
		ChildTasks: childTasks,
	}, nil
}

func decodeStartRunPlannerTask(obj map[string]any, index int) (orchestration.PlannedTaskSpec, error) {
	if err := rejectUnknownPlannerKeys(obj, map[string]struct{}{
		"alias":               {},
		"kind":                {},
		"goal":                {},
		"inputs":              {},
		"depends_on":          {},
		"worker_profile":      {},
		"priority":            {},
		"retry_policy":        {},
		"verification_policy": {},
		"blackboard_scope":    {},
	}); err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	goal, err := requiredPlannerString(obj, "goal")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	alias, err := optionalPlannerString(obj, "alias")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	kind, err := optionalPlannerString(obj, "kind")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	workerProfile, err := optionalPlannerString(obj, "worker_profile")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	blackboardScope, err := optionalPlannerString(obj, "blackboard_scope")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	dependsOn, err := optionalPlannerStringArray(obj, "depends_on")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	priority, err := optionalPlannerInt(obj, "priority")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	inputs, err := optionalPlannerObject(obj, "inputs")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	retryPolicy, err := optionalPlannerObject(obj, "retry_policy")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	verificationPolicy, err := optionalPlannerObject(obj, "verification_policy")
	if err != nil {
		return orchestration.PlannedTaskSpec{}, fmt.Errorf("child_tasks[%d]: %w", index, err)
	}
	return orchestration.PlannedTaskSpec{
		Alias:              alias,
		Kind:               kind,
		Goal:               goal,
		Inputs:             inputs,
		DependsOnAliases:   dependsOn,
		WorkerProfile:      workerProfile,
		Priority:           priority,
		RetryPolicy:        retryPolicy,
		VerificationPolicy: verificationPolicy,
		BlackboardScope:    blackboardScope,
	}, nil
}

func rejectUnknownPlannerKeys(obj map[string]any, allowed map[string]struct{}) error {
	for key := range obj {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

func requiredPlannerString(obj map[string]any, key string) (string, error) {
	value, err := optionalPlannerString(obj, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optionalPlannerString(obj map[string]any, key string) (string, error) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(value), nil
}

func optionalPlannerObject(obj map[string]any, key string) (map[string]any, error) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return map[string]any{}, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return normalizeObject(value), nil
}

func optionalPlannerStringArray(obj map[string]any, key string) ([]string, error) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	values := make([]string, 0, len(items))
	for index, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", key, index)
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil, fmt.Errorf("%s[%d] must not be empty", key, index)
		}
		values = append(values, trimmed)
	}
	return values, nil
}

func optionalPlannerInt(obj map[string]any, key string) (int, error) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return 0, nil
	}
	switch value := raw.(type) {
	case int:
		return value, nil
	case int32:
		return int(value), nil
	case int64:
		return int(value), nil
	case float64:
		integer := int(value)
		if value != float64(integer) {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return integer, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
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
- Use exactly the JSON field names shown below. Do not invent aliases or extra fields.
- Return child_tasks as an array. Use [] when no decomposition is needed.

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
