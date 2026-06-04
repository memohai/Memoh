package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/session"
)

type sessionCreateQueries struct {
	dbstore.Queries
	bot          sqlc.GetBotByIDRow
	createCalled bool
	createParams sqlc.CreateSessionParams
}

func (q *sessionCreateQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (*sessionCreateQueries) ListSessionsByBot(_ context.Context, _ pgtype.UUID) ([]sqlc.ListSessionsByBotRow, error) {
	return nil, nil
}

func (q *sessionCreateQueries) CreateSession(_ context.Context, arg sqlc.CreateSessionParams) (sqlc.BotSession, error) {
	q.createCalled = true
	q.createParams = arg
	return sqlc.BotSession{
		ID:          testUUID("22222222-2222-2222-2222-222222222222"),
		BotID:       arg.BotID,
		ChannelType: arg.ChannelType,
		Type:        arg.Type,
		Title:       arg.Title,
		Metadata:    arg.Metadata,
		CreatedAt:   pgtype.Timestamptz{Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Valid: true},
	}, nil
}

func TestCreateSessionRejectsUnknownTypeAsBadRequest(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionCreateQueries{
		bot: testBotRow(botID, map[string]any{}),
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	err := callCreateSession(handler, botID, `{"type":"conversation","title":"bad"}`)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("CreateSession() error = %v, want HTTP 400", err)
	}
	if queries.createCalled {
		t.Fatalf("CreateSession should reject unknown type before DB insert")
	}
}

func TestCreateSessionAcceptsACPAgentType(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionCreateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	body := `{"type":"acp_agent","title":"Codex","metadata":{"acp_agent_id":"codex","project_path":"/data/app"}}`
	if err := callCreateSession(handler, botID, body); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if !queries.createCalled {
		t.Fatalf("CreateSession did not insert ACP session")
	}
	if queries.createParams.Type != session.TypeACPAgent {
		t.Fatalf("CreateSession type = %q, want acp_agent", queries.createParams.Type)
	}
	if got := string(queries.createParams.Metadata); !strings.Contains(got, `"acp_agent_id":"codex"`) || !strings.Contains(got, `"project_path":"/data/app"`) {
		t.Fatalf("CreateSession metadata = %s", got)
	}
}

func TestCreateSessionDefaultsACPProjectPath(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionCreateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	body := `{"type":"acp_agent","title":"Codex","metadata":{"acp_agent_id":"codex"}}`
	if err := callCreateSession(handler, botID, body); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(queries.createParams.Metadata, &metadata); err != nil {
		t.Fatalf("metadata json = %v", err)
	}
	if metadata["project_path"] != session.DefaultACPProjectPath || metadata["acp_project_mode"] != session.DefaultACPProjectMode {
		t.Fatalf("CreateSession metadata = %#v, want default ACP project", metadata)
	}
}

type recordingRuntimeBinder struct {
	bindArgs []string
	bindErr  error
}

func (*recordingRuntimeBinder) CloseSession(string) error { return nil }

func (b *recordingRuntimeBinder) BindRuntime(botID, runtimeID, sessionID, agentID, projectPath string) error {
	b.bindArgs = []string{botID, runtimeID, sessionID, agentID, projectPath}
	return b.bindErr
}

func TestCreateSessionBindsWarmACPRuntime(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionCreateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
	}
	binder := &recordingRuntimeBinder{}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries),
		binder,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	body := `{"type":"acp_agent","title":"Codex","metadata":{"acp_agent_id":"codex"},"acp_runtime_id":"rt_warm"}`
	if err := callCreateSession(handler, botID, body); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	want := []string{botID, "rt_warm", "22222222-2222-2222-2222-222222222222", "codex", session.DefaultACPProjectPath}
	if len(binder.bindArgs) != len(want) {
		t.Fatalf("bind args = %#v, want %#v", binder.bindArgs, want)
	}
	for i := range want {
		if binder.bindArgs[i] != want[i] {
			t.Fatalf("bind args = %#v, want %#v", binder.bindArgs, want)
		}
	}
}

func TestCreateSessionToleratesFailedRuntimeBind(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionCreateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
	}
	binder := &recordingRuntimeBinder{bindErr: errors.New("runtime gone")}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries),
		binder,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	// A failed bind must not fail session creation: the first prompt cold
	// starts instead.
	body := `{"type":"acp_agent","title":"Codex","metadata":{"acp_agent_id":"codex"},"acp_runtime_id":"rt_gone"}`
	if err := callCreateSession(handler, botID, body); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if !queries.createCalled {
		t.Fatalf("session was not created")
	}
}

func callCreateSession(handler *SessionHandler, botID string, body string) error {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/bots/"+botID+"/sessions", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	ctx.SetPath("/bots/:bot_id/sessions")
	ctx.SetParamNames("bot_id")
	ctx.SetParamValues(botID)
	return handler.CreateSession(ctx)
}
