package toolapproval

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/settings"
)

type targetPolicyResolverStub struct {
	requested string
	policy    WorkspaceTargetPolicy
	err       error
}

func (s *targetPolicyResolverStub) ResolveWorkspaceTargetPolicy(_ context.Context, _ string, targetID string) (WorkspaceTargetPolicy, error) {
	s.requested = targetID
	return s.policy, s.err
}

func TestEvaluatePolicyUsesTargetConfigAndPinsCanonicalTarget(t *testing.T) {
	resolver := &targetPolicyResolverStub{policy: WorkspaceTargetPolicy{
		TargetID: "canonical-target",
		Kind:     "remote",
		Name:     "Office Mac",
		Config: settings.ToolApprovalConfig{
			Enabled: true,
			Write:   settings.ToolApprovalFilePolicy{Mode: settings.ToolApprovalDeny},
		},
	}}
	service := NewService(nil, nil, nil)
	service.SetWorkspaceTargetPolicyResolver(resolver)
	input := map[string]any{"target_id": "requested-target", "path": "notes.txt"}

	evaluation, err := service.EvaluatePolicy(context.Background(), CreatePendingInput{
		BotID:             "bot-1",
		ToolName:          "write",
		ToolInput:         input,
		WorkspaceTargeted: true,
	})
	if err != nil {
		t.Fatalf("EvaluatePolicy() error = %v", err)
	}
	if evaluation.Decision != DecisionDeny {
		t.Fatalf("decision = %q, want %q", evaluation.Decision, DecisionDeny)
	}
	if resolver.requested != "requested-target" {
		t.Fatalf("resolver target = %q", resolver.requested)
	}
	if got := input["target_id"]; got != "canonical-target" {
		t.Fatalf("canonical target_id = %#v", got)
	}
	if evaluation.ExecutionLocation == nil {
		t.Fatal("execution location is nil")
	}
	if got := *evaluation.ExecutionLocation; got.TargetID != "canonical-target" || got.Kind != "remote" || got.Name != "Office Mac" {
		t.Fatalf("execution location = %#v", got)
	}
}

func TestEvaluatePolicyPinsPrimaryWhenTargetIsOmitted(t *testing.T) {
	resolver := &targetPolicyResolverStub{policy: WorkspaceTargetPolicy{
		TargetID: "primary-at-approval-time",
		Config: settings.ToolApprovalConfig{
			Enabled: true,
			Exec:    settings.ToolApprovalExecPolicy{Mode: settings.ToolApprovalAsk},
		},
	}}
	service := NewService(nil, nil, nil)
	service.SetWorkspaceTargetPolicyResolver(resolver)
	input := map[string]any{"command": "make test"}

	evaluation, err := service.EvaluatePolicy(context.Background(), CreatePendingInput{
		BotID:             "bot-1",
		ToolName:          "exec",
		ToolInput:         input,
		WorkspaceTargeted: true,
	})
	if err != nil {
		t.Fatalf("EvaluatePolicy() error = %v", err)
	}
	if evaluation.Decision != DecisionNeedsApproval {
		t.Fatalf("decision = %q, want %q", evaluation.Decision, DecisionNeedsApproval)
	}
	if resolver.requested != "" || input["target_id"] != "primary-at-approval-time" {
		t.Fatalf("requested/canonical target = %q/%#v", resolver.requested, input["target_id"])
	}
}

func TestEvaluatePolicyReturnsTargetResolutionError(t *testing.T) {
	want := errors.New("remote runtime is offline")
	service := NewService(nil, nil, nil)
	service.SetWorkspaceTargetPolicyResolver(&targetPolicyResolverStub{err: want})
	_, err := service.EvaluatePolicy(context.Background(), CreatePendingInput{
		BotID:             "bot-1",
		ToolName:          "read",
		ToolInput:         map[string]any{"path": "README.md"},
		WorkspaceTargeted: true,
	})
	if !errors.Is(err, want) {
		t.Fatalf("EvaluatePolicy() error = %v, want %v", err, want)
	}
}
