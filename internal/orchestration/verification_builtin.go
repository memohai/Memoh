package orchestration

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type builtinVerificationPolicy struct {
	Mode                    string
	RequireStructuredOutput bool
	RequiredArtifactKinds   []string
	OnReject                string
}

func ExecuteBuiltinVerification(ctx context.Context, queries *sqlc.Queries, verification TaskVerification) VerificationCompletion {
	pgTaskID, err := db.ParseUUID(verification.TaskID)
	if err != nil {
		return failedVerificationCompletion(verification, "task_lookup_failed", fmt.Sprintf("invalid task id %q", verification.TaskID))
	}
	pgResultID, err := db.ParseUUID(verification.ResultID)
	if err != nil {
		return failedVerificationCompletion(verification, "result_lookup_failed", fmt.Sprintf("invalid result id %q", verification.ResultID))
	}

	taskRow, err := queries.GetOrchestrationTaskByID(ctx, pgTaskID)
	if err != nil {
		return failedVerificationCompletion(verification, "task_lookup_failed", fmt.Sprintf("load task: %v", err))
	}
	resultRow, err := queries.GetOrchestrationTaskResultByID(ctx, pgResultID)
	if err != nil {
		return failedVerificationCompletion(verification, "result_lookup_failed", fmt.Sprintf("load result: %v", err))
	}
	artifacts, err := queries.ListOrchestrationArtifactsByTask(ctx, taskRow.ID)
	if err != nil {
		return failedVerificationCompletion(verification, "artifact_lookup_failed", fmt.Sprintf("load artifacts: %v", err))
	}

	policy := decodeJSONObject(taskRow.VerificationPolicy)
	structuredOutput := decodeJSONObject(resultRow.StructuredOutput)
	return evaluateBuiltinVerification(verification, policy, structuredOutput, artifactsForResultAttempt(artifacts, resultRow.AttemptID))
}

func evaluateBuiltinVerification(verification TaskVerification, policy map[string]any, structuredOutput map[string]any, artifacts []sqlc.OrchestrationArtifact) VerificationCompletion {
	normalized, err := normalizeVerificationPolicy(policy)
	if err != nil {
		return failedVerificationCompletion(verification, "invalid_verification_policy", err.Error())
	}

	var failures []string
	if normalized.RequireStructuredOutput && len(structuredOutput) == 0 {
		failures = append(failures, "structured_output is required")
	}

	for _, kind := range normalized.RequiredArtifactKinds {
		if !hasArtifactKind(artifacts, kind) {
			failures = append(failures, fmt.Sprintf("artifact kind %q is required", kind))
		}
	}

	if len(failures) == 0 {
		return VerificationCompletion{
			VerificationID: verification.ID,
			ClaimToken:     verification.ClaimToken,
			Status:         TaskVerificationStatusCompleted,
			Verdict:        VerificationVerdictAccepted,
			Summary:        "builtin verification passed",
			FailureClass:   "",
			TerminalReason: "",
			RequestReplan:  false,
		}
	}

	requestReplan := normalized.OnReject == VerificationRejectActionReplan
	if requestReplan && !hasReplacementPlan(structuredOutput) {
		requestReplan = false
		failures = append(failures, "replacement plan child_tasks is required for request_replan")
	}

	status := TaskVerificationStatusFailed
	failureClass := "verification_rejected"
	if requestReplan {
		status = TaskVerificationStatusCompleted
		failureClass = "verification_requested_replan"
	}
	summary := strings.Join(failures, "; ")
	return VerificationCompletion{
		VerificationID: verification.ID,
		ClaimToken:     verification.ClaimToken,
		Status:         status,
		Verdict:        VerificationVerdictRejected,
		Summary:        summary,
		FailureClass:   failureClass,
		TerminalReason: summary,
		RequestReplan:  requestReplan,
	}
}

func normalizeVerificationPolicy(raw map[string]any) (builtinVerificationPolicy, error) {
	policy := builtinVerificationPolicy{
		Mode:     VerificationModeBuiltinBasic,
		OnReject: VerificationRejectActionFailTask,
	}
	if len(raw) == 0 {
		return policy, nil
	}

	if mode := strings.TrimSpace(stringValue(raw["mode"])); mode != "" {
		if mode != VerificationModeBuiltinBasic {
			return builtinVerificationPolicy{}, fmt.Errorf("unsupported verification mode %q", mode)
		}
		policy.Mode = mode
	}
	if requireStructuredOutput, ok := raw["require_structured_output"].(bool); ok {
		policy.RequireStructuredOutput = requireStructuredOutput
	}
	policy.RequiredArtifactKinds = normalizeStringArray(raw["required_artifact_kinds"])
	if onReject := strings.TrimSpace(stringValue(raw["on_reject"])); onReject != "" {
		switch onReject {
		case VerificationRejectActionFailTask, VerificationRejectActionReplan:
			policy.OnReject = onReject
		default:
			return builtinVerificationPolicy{}, fmt.Errorf("unsupported verification on_reject %q", onReject)
		}
	}
	return policy, nil
}

func verificationRejectAction(raw map[string]any) string {
	policy, err := normalizeVerificationPolicy(raw)
	if err != nil {
		return VerificationRejectActionFailTask
	}
	return policy.OnReject
}

func artifactsForResultAttempt(artifacts []sqlc.OrchestrationArtifact, attemptID pgtype.UUID) []sqlc.OrchestrationArtifact {
	if !attemptID.Valid {
		return nil
	}
	filtered := make([]sqlc.OrchestrationArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.AttemptID.Valid && artifact.AttemptID == attemptID {
			filtered = append(filtered, artifact)
		}
	}
	return filtered
}

func hasArtifactKind(artifacts []sqlc.OrchestrationArtifact, kind string) bool {
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Kind) == kind {
			return true
		}
	}
	return false
}

func hasReplacementPlan(structuredOutput map[string]any) bool {
	childTasks, ok := structuredOutput["child_tasks"].([]any)
	return ok && len(childTasks) > 0
}

func failedVerificationCompletion(verification TaskVerification, failureClass, reason string) VerificationCompletion {
	return VerificationCompletion{
		VerificationID: verification.ID,
		ClaimToken:     verification.ClaimToken,
		Status:         TaskVerificationStatusFailed,
		Verdict:        VerificationVerdictRejected,
		Summary:        reason,
		FailureClass:   failureClass,
		TerminalReason: reason,
		RequestReplan:  false,
	}
}

func normalizeStringArray(raw any) []string {
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
	return values
}
