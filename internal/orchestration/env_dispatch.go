package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// envCapture is the in-memory bookkeeping the dispatch path threads through
// the manifest and into the post-commit release/hold paths. The kernel
// transcribes it into the captured_env_preconditions JSONB column so a
// verifier replay or a delayed release can reconstruct the lease without
// re-resolving the planner-supplied resource_name.
type envCapture struct {
	Kind          string
	ResourceID    string
	ResourceName  string
	SessionID     string
	LeaseToken    string
	LeaseEpoch    int64
	BindingID     string
	RuntimeHandle map[string]any
	Mode          string
	EffectClass   string
	Metadata      map[string]any
}

// acquireEnvForDispatch reserves a session and resolves the planner-supplied
// resource_name when env_preconditions.required=true. The kernel only calls
// the env manager from inside the candidate loop after the task row is locked
// FOR UPDATE, so two schedulers cannot acquire two sessions for the same
// task. The returned *envCapture is nil when the task does not need an env;
// the dispatch path treats that as the common-case fast path.
func (s *Service) acquireEnvForDispatch(ctx context.Context, runRow sqlc.OrchestrationRun, taskRow sqlc.OrchestrationTask, pre EnvPreconditions) (*envCapture, error) {
	if !pre.Required {
		return nil, nil
	}
	if s.envManager == nil {
		return nil, fmt.Errorf("orchestration: task %s declares env_preconditions but env manager is not configured", taskRow.ID.String())
	}
	if pre.Kind == "" || pre.ResourceName == "" {
		return nil, fmt.Errorf("orchestration: task %s env_preconditions missing kind or resource_name", taskRow.ID.String())
	}
	resource, err := s.envManager.GetEnvResourceByName(ctx, runRow.TenantID, pre.ResourceName)
	if err != nil {
		return nil, fmt.Errorf("orchestration: resolve env resource %q: %w", pre.ResourceName, err)
	}
	if resource.Status != "" && resource.Status != "active" {
		return nil, fmt.Errorf("orchestration: env resource %q is %s, refusing dispatch", pre.ResourceName, resource.Status)
	}
	if resource.Kind != pre.Kind {
		return nil, fmt.Errorf("orchestration: env resource %q has kind %q but planner requested %q", pre.ResourceName, resource.Kind, pre.Kind)
	}
	leaseTTL := s.envLeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultEnvLeaseTTL
	}
	lease, err := s.envManager.AcquireEnvSession(ctx, EnvAcquireRequest{
		TenantID:        runRow.TenantID,
		ResourceID:      resource.ID,
		LeaseHolderKind: EnvLeaseHolderWorker,
		LeaseHolderID:   "kernel-dispatch",
		LeaseTTL:        leaseTTL,
		RunID:           taskRow.RunID.String(),
		TaskID:          taskRow.ID.String(),
		Metadata:        pre.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("orchestration: acquire env session: %w", err)
	}
	return &envCapture{
		Kind:          resource.Kind,
		ResourceID:    resource.ID,
		ResourceName:  resource.Name,
		SessionID:     lease.SessionID,
		LeaseToken:    lease.LeaseToken,
		LeaseEpoch:    lease.LeaseEpoch,
		RuntimeHandle: lease.RuntimeHandle,
		Mode:          pre.Mode,
		EffectClass:   pre.EffectClass,
		Metadata:      pre.Metadata,
	}, nil
}

// bindEnvAndPersistCapture creates the session→attempt binding and updates
// the input manifest with the binding id. Both writes go through qtx so the
// dispatch transaction stays atomic; the env manager's session row already
// exists outside qtx (acquired earlier) and gets reconciled by the reclaim
// loop if the dispatch tx ultimately rolls back.
func (s *Service) bindEnvAndPersistCapture(ctx context.Context, qtx *sqlc.Queries, tenantID string, manifestID pgtype.UUID, attempt sqlc.OrchestrationTaskAttempt, capture *envCapture) error {
	if capture == nil {
		return nil
	}
	if s.envManager == nil {
		return errors.New("orchestration: env manager became unavailable mid-dispatch")
	}
	binding, err := s.envManager.CreateEnvBinding(ctx, EnvCreateBindingRequest{
		SessionID:  capture.SessionID,
		LeaseToken: capture.LeaseToken,
		LeaseEpoch: capture.LeaseEpoch,
		RunID:      attempt.RunID.String(),
		TaskID:     attempt.TaskID.String(),
		AttemptID:  attempt.ID.String(),
		Purpose:    EnvBindingPurposePrimary,
		Metadata:   capture.Metadata,
	})
	if err != nil {
		return fmt.Errorf("orchestration: create env binding: %w", err)
	}
	capture.BindingID = binding.BindingID
	if err := qtx.UpdateOrchestrationInputManifestEnvCapture(ctx, sqlc.UpdateOrchestrationInputManifestEnvCaptureParams{
		ID:                       manifestID,
		CapturedEnvPreconditions: marshalCapturedEnvPreconditions(capture, EnvPreconditions{Required: true}),
	}); err != nil {
		return fmt.Errorf("orchestration: persist env capture on manifest: %w", err)
	}
	_ = tenantID
	return nil
}

// releaseEnvAfterFailedDispatch is the compensating action when the kernel
// decides not to proceed with a dispatch after acquiring an env session.
// Errors from the env manager are logged rather than surfaced because the
// caller is already returning a different error; the reclaim sweep is the
// authoritative backstop for any state that does not get released here.
func (s *Service) releaseEnvAfterFailedDispatch(ctx context.Context, capture *envCapture, reason string) {
	if capture == nil || s.envManager == nil {
		return
	}
	if capture.BindingID != "" {
		if err := s.envManager.ReleaseEnvBinding(ctx, EnvReleaseBindingRequest{
			BindingID:  capture.BindingID,
			LeaseToken: capture.LeaseToken,
			LeaseEpoch: capture.LeaseEpoch,
			Reason:     reason,
		}); err != nil {
			s.logger.Warn("release env binding after dispatch abort failed",
				slog.String("binding_id", capture.BindingID),
				slog.String("reason", reason),
				slog.Any("error", err),
			)
		}
	}
	if err := s.envManager.ReleaseEnvSession(ctx, EnvReleaseSessionRequest{
		SessionID:  capture.SessionID,
		LeaseToken: capture.LeaseToken,
		LeaseEpoch: capture.LeaseEpoch,
		Reason:     reason,
	}); err != nil {
		s.logger.Warn("release env session after dispatch abort failed",
			slog.String("session_id", capture.SessionID),
			slog.String("reason", reason),
			slog.Any("error", err),
		)
	}
}

// releaseEnvForAttemptCommitFailure runs after a dispatch transaction commit
// itself fails. The attempt row will not exist in Postgres but the env
// session/binding might. We look up the manifest envelope from in-memory
// state we already have on the attempt row and ask the env manager to
// release. The reclaim loop is the safety net if this best-effort release
// loses the race.
func (s *Service) releaseEnvForAttemptCommitFailure(ctx context.Context, attempt sqlc.OrchestrationTaskAttempt) {
	if s.envManager == nil {
		return
	}
	manifestRow, err := s.queries.GetOrchestrationInputManifestByID(ctx, attempt.InputManifestID)
	if err != nil {
		// The manifest never made it to storage either; nothing to release.
		return
	}
	capture, ok := decodeCapturedEnvPreconditions(manifestRow.CapturedEnvPreconditions)
	if !ok {
		return
	}
	s.releaseEnvAfterFailedDispatch(ctx, capture, "dispatch_commit_failed")
}

// envCaptureForHash returns the deterministic projection of env state that
// feeds the manifest projection_hash. We hash the resource identity and the
// lease fencing payload but deliberately omit ephemeral fields like the
// runtime handle and metadata so a verifier replay agrees with a fresh
// dispatch on the same logical request.
func envCaptureForHash(capture *envCapture, pre EnvPreconditions) map[string]any {
	if capture == nil {
		return map[string]any{"required": pre.Required}
	}
	return map[string]any{
		"required":      true,
		"kind":          capture.Kind,
		"resource_id":   capture.ResourceID,
		"resource_name": capture.ResourceName,
		"session_id":    capture.SessionID,
		"lease_epoch":   capture.LeaseEpoch,
		"mode":          capture.Mode,
		"effect_class":  capture.EffectClass,
	}
}

// marshalCapturedEnvPreconditions builds the JSONB payload the kernel
// persists in orchestration_input_manifests.captured_env_preconditions. For
// pure-LLM tasks it falls through to the same sentinel S3-E.1 introduced so
// the bytes stay byte-identical with the column default.
func marshalCapturedEnvPreconditions(capture *envCapture, pre EnvPreconditions) []byte {
	if capture == nil && !pre.Required {
		return defaultEnvPreconditionsJSON()
	}
	envelope := map[string]any{"required": true}
	if capture == nil {
		envelope["kind"] = pre.Kind
		envelope["resource_name"] = pre.ResourceName
		if pre.Mode != "" {
			envelope["mode"] = pre.Mode
		}
		if pre.EffectClass != "" {
			envelope["effect_class"] = pre.EffectClass
		}
		return marshalJSON(envelope)
	}
	envelope["kind"] = capture.Kind
	envelope["resource_id"] = capture.ResourceID
	envelope["resource_name"] = capture.ResourceName
	envelope["session_id"] = capture.SessionID
	envelope["lease_token"] = capture.LeaseToken
	envelope["lease_epoch"] = capture.LeaseEpoch
	if capture.BindingID != "" {
		envelope["binding_id"] = capture.BindingID
	}
	if capture.Mode != "" {
		envelope["mode"] = capture.Mode
	}
	if capture.EffectClass != "" {
		envelope["effect_class"] = capture.EffectClass
	}
	if len(capture.RuntimeHandle) > 0 {
		envelope["runtime_handle"] = capture.RuntimeHandle
	}
	if len(capture.Metadata) > 0 {
		envelope["metadata"] = capture.Metadata
	}
	return marshalJSON(envelope)
}

// decodeCapturedEnvPreconditions returns the *envCapture rebuilt from a
// manifest row. It returns ok=false for the not-required case so the release
// path can short-circuit without a special branch.
func decodeCapturedEnvPreconditions(data []byte) (*envCapture, bool) {
	if len(data) == 0 {
		return nil, false
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false
	}
	required, _ := raw["required"].(bool)
	if !required {
		return nil, false
	}
	sessionID, _ := raw["session_id"].(string)
	leaseToken, _ := raw["lease_token"].(string)
	if sessionID == "" || leaseToken == "" {
		// Required but the dispatch path never persisted a real lease.
		// Nothing to release.
		return nil, false
	}
	capture := &envCapture{
		Kind:         stringFrom(raw, "kind"),
		ResourceID:   stringFrom(raw, "resource_id"),
		ResourceName: stringFrom(raw, "resource_name"),
		SessionID:    sessionID,
		LeaseToken:   leaseToken,
		BindingID:    stringFrom(raw, "binding_id"),
		Mode:         stringFrom(raw, "mode"),
		EffectClass:  stringFrom(raw, "effect_class"),
	}
	if epoch, ok := raw["lease_epoch"].(float64); ok {
		capture.LeaseEpoch = int64(epoch)
	}
	if handle, ok := raw["runtime_handle"].(map[string]any); ok && len(handle) > 0 {
		capture.RuntimeHandle = handle
	}
	if metadata, ok := raw["metadata"].(map[string]any); ok && len(metadata) > 0 {
		capture.Metadata = metadata
	}
	return capture, true
}

func stringFrom(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// releaseEnvForAttempt is invoked after the kernel commits a terminal
// attempt completion (success or failure). It rebuilds the envelope from the
// manifest the dispatcher captured and asks the env manager to release the
// binding and the underlying session. HITL paths that move a task to
// waiting_human go through holdEnvForAttempt instead so the session stays
// warm for resume.
func (s *Service) releaseEnvForAttempt(ctx context.Context, manifestID pgtype.UUID, reason string) {
	if s.envManager == nil {
		return
	}
	manifestRow, err := s.queries.GetOrchestrationInputManifestByID(ctx, manifestID)
	if err != nil {
		return
	}
	capture, ok := decodeCapturedEnvPreconditions(manifestRow.CapturedEnvPreconditions)
	if !ok {
		return
	}
	s.releaseEnvAfterFailedDispatch(ctx, capture, reason)
}
