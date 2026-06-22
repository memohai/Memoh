package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/agent/sessionmode"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
)

func TestIsInteractiveApprovalSession(t *testing.T) {
	t.Parallel()

	for _, sessionType := range []string{"", sessionmode.Chat, "CHAT", sessionmode.ACPAgent} {
		if !isInteractiveApprovalSession(sessionType) {
			t.Fatalf("expected %q to allow interactive approvals", sessionType)
		}
	}

	for _, sessionType := range []string{sessionmode.Discuss, sessionmode.Schedule, sessionmode.Heartbeat, sessionmode.Subagent} {
		if isInteractiveApprovalSession(sessionType) {
			t.Fatalf("expected %q to reject interactive approvals", sessionType)
		}
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
