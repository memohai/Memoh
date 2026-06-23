package flow

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/sessionmode"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

func TestIsInteractiveApprovalSession(t *testing.T) {
	t.Parallel()

	for _, sessionType := range []string{"", sessionmode.Chat, "CHAT", sessionmode.ACPAgent} {
		if !isInteractiveApprovalSession(sessionType) {
			t.Fatalf("expected %q to allow interactive approvals", sessionType)
		}
	}

	for _, sessionType := range []string{sessionmode.Discuss, sessionmode.Schedule, sessionmode.Heartbeat, sessionmode.Subagent, sessionmode.BackgroundDelivery} {
		if isInteractiveApprovalSession(sessionType) {
			t.Fatalf("expected %q to reject interactive approvals", sessionType)
		}
	}
}

func TestToolApprovalHandlerRejectsAskUserBeforeCreateInBackgroundDelivery(t *testing.T) {
	t.Parallel()

	fake := &fakeUserInputService{}
	resolver := &Resolver{userInput: fake}
	handler := resolver.buildToolApprovalHandler(baseRunConfigParams{
		BotID:       "bot-1",
		SessionID:   "session-1",
		SessionType: sessionmode.BackgroundDelivery,
	})

	result, err := handler(context.Background(), sdk.ToolCall{
		ToolCallID: "call-1",
		ToolName:   userinput.ToolNameAskUser,
		Input: map[string]any{
			"questions": []any{
				map[string]any{
					"text":    "Continue?",
					"kind":    userinput.QuestionKindSingleSelect,
					"options": []any{map[string]any{"label": "Yes"}, map[string]any{"label": "No"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.Decision != sdk.ToolApprovalDecisionRejected {
		t.Fatalf("decision = %q, want rejected", result.Decision)
	}
	if fake.createCalls != 0 || fake.cancelCalls != 0 {
		t.Fatalf("background ask_user should reject before persistence, create=%d cancel=%d", fake.createCalls, fake.cancelCalls)
	}
}

func TestToolApprovalHandlerLimitsForcedApprovalRejectionReason(t *testing.T) {
	t.Parallel()

	large := "HEAD\n" + strings.Repeat("rejected detail ", 300) + "\nTAIL"
	resolver := &Resolver{
		agent: agentpkg.New(agentpkg.Deps{
			Limits: agentpkg.Limits{ToolOutputMaxBytes: 512, ToolOutputMaxLines: 80},
		}),
	}
	handler := resolver.buildToolApprovalHandler(baseRunConfigParams{
		BotID:       "bot-1",
		SessionID:   "session-1",
		SessionType: sessionmode.Chat,
	})

	result, err := handler(agentpkg.ContextWithHookForcedApproval(context.Background(), large), sdk.ToolCall{
		ToolCallID: "call-1",
		ToolName:   "write",
		Input:      map[string]any{},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.Decision != sdk.ToolApprovalDecisionRejected {
		t.Fatalf("decision = %q, want rejected", result.Decision)
	}
	if len(result.Reason) >= len(large) {
		t.Fatalf("approval reason was not pruned: got %d bytes, original %d", len(result.Reason), len(large))
	}
	if !strings.Contains(result.Reason, "[memoh pruned]") {
		t.Fatalf("approval reason missing prune marker:\n%s", result.Reason)
	}
}

func TestAgentSessionModesMatchPersistedSessionTypes(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		sessionmode.Chat:      session.TypeChat,
		sessionmode.Heartbeat: session.TypeHeartbeat,
		sessionmode.Schedule:  session.TypeSchedule,
		sessionmode.Subagent:  session.TypeSubagent,
		sessionmode.Discuss:   session.TypeDiscuss,
		sessionmode.ACPAgent:  session.TypeACPAgent,
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("agent session mode %q must match persisted type %q", got, want)
		}
	}
}

func TestResolveRunConfigSessionTypeUsesStoredSessionType(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				if sessionID != "session-1" {
					t.Fatalf("unexpected session id: %s", sessionID)
				}
				return session.Session{ID: sessionID, Type: session.TypeChat}, nil
			},
		},
	}

	if got := resolver.resolveRunConfigSessionType(context.Background(), "session-1"); got != session.TypeChat {
		t.Fatalf("session type = %q, want %q", got, session.TypeChat)
	}
}

func TestResolveRunConfigSessionTypeFallsBackToChat(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		sessionService: &fakeBackgroundSessionService{
			getFn: func(context.Context, string) (session.Session, error) {
				return session.Session{}, errors.New("db unavailable")
			},
		},
	}

	if got := resolver.resolveRunConfigSessionType(context.Background(), "session-1"); got != session.TypeChat {
		t.Fatalf("session type = %q, want %q", got, session.TypeChat)
	}
}

func TestApprovalResultMetadata(t *testing.T) {
	t.Parallel()

	got := approvalResultMetadata(toolapproval.Request{
		ShortID:    7,
		Status:     toolapproval.StatusRejected,
		ToolName:   "exec",
		ToolCallID: "call-1",
	})

	if got["short_id"] != 7 ||
		got["status"] != toolapproval.StatusRejected ||
		got["tool_name"] != "exec" ||
		got["tool_call_id"] != "call-1" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
}

func TestResolverLimitToolResultTextUsesAgentLimits(t *testing.T) {
	t.Parallel()

	r := &Resolver{
		agent: agentpkg.New(agentpkg.Deps{
			Limits: agentpkg.Limits{ToolOutputMaxBytes: 512, ToolOutputMaxLines: 80},
		}),
	}
	large := "HEAD\n" + strings.Repeat("rejected detail ", 300) + "\nTAIL"

	got := r.limitToolResultText(large, "write")
	if len(got) >= len(large) {
		t.Fatalf("tool result text was not pruned: got %d bytes, original %d", len(got), len(large))
	}
	if !strings.Contains(got, "[memoh pruned]") {
		t.Fatalf("tool result text missing prune marker:\n%s", got)
	}
}
