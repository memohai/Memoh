package flow

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/sessionmode"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/toolapproval"
)

type denyToolApprovalSettingsQueries struct {
	dbstore.Queries
}

func (*denyToolApprovalSettingsQueries) GetSettingsByBotID(_ context.Context, botID pgtype.UUID) (sqlc.GetSettingsByBotIDRow, error) {
	return sqlc.GetSettingsByBotIDRow{
		BotID:              botID,
		ToolApprovalConfig: []byte(`{"read":{"mode":"deny"}}`),
	}, nil
}

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

func TestToolApprovalPolicyDenyWinsOverHookForcedApproval(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	settingsService := settings.NewService(log, &denyToolApprovalSettingsQueries{}, nil, nil)
	approvalService := toolapproval.NewService(log, nil, settingsService)
	resolver := &Resolver{toolApproval: approvalService}
	handler := resolver.buildToolApprovalHandler(baseRunConfigParams{
		BotID:       "11111111-1111-1111-1111-111111111111",
		SessionID:   "22222222-2222-2222-2222-222222222222",
		SessionType: sessionmode.Chat,
	})

	result, err := handler(agentpkg.ContextWithHookForcedApproval(context.Background(), "hook asks for review"), sdk.ToolCall{
		ToolCallID: "call-1",
		ToolName:   "read",
		Input:      map[string]any{"path": "/data/file.txt"},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.Decision != sdk.ToolApprovalDecisionRejected || result.Reason != toolapproval.PolicyDeniedReason {
		t.Fatalf("result = %+v, want policy rejection", result)
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

func TestResolveRunConfigSkipsModelResolutionForACPRuntime(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				if sessionID != "session-1" {
					t.Fatalf("unexpected session id: %s", sessionID)
				}
				return session.Session{
					ID:          sessionID,
					Type:        session.TypeDiscuss,
					SessionMode: session.TypeDiscuss,
					RuntimeType: session.RuntimeACPAgent,
				}, nil
			},
		},
	}

	got, err := resolver.ResolveRunConfig(context.Background(), "bot-1", "session-1", "user-1", "telegram", "", "group", "")
	if err != nil {
		t.Fatalf("ResolveRunConfig() error = %v", err)
	}
	if got.RuntimeType != session.RuntimeACPAgent {
		t.Fatalf("runtime type = %q, want %q", got.RuntimeType, session.RuntimeACPAgent)
	}
	if got.RunConfig.SessionType != session.TypeDiscuss {
		t.Fatalf("run config session type = %q, want %q", got.RunConfig.SessionType, session.TypeDiscuss)
	}
	if got.ModelID != "" || got.RunConfig.Model != nil {
		t.Fatalf("ACP runtime should not resolve a model, model_id=%q model=%#v", got.ModelID, got.RunConfig.Model)
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
