package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/session"
)

type sessionUpdateQueries struct {
	dbstore.Queries
	bot          sqlc.GetBotByIDRow
	session      sqlc.BotSession
	messageCount int64

	updateCalled bool
	updateParams sqlc.UpdateSessionTypeAndMetadataParams

	titleUpdateCalled bool
}

func (q *sessionUpdateQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q *sessionUpdateQueries) GetSessionByID(_ context.Context, _ pgtype.UUID) (sqlc.BotSession, error) {
	return q.session, nil
}

func (*sessionUpdateQueries) ListBotUserGrantsForUser(_ context.Context, _ sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error) {
	return []sqlc.ListBotUserGrantsForUserRow{{Permissions: []byte(`["chat"]`)}}, nil
}

func (*sessionUpdateQueries) ListSessionsByBot(_ context.Context, _ pgtype.UUID) ([]sqlc.ListSessionsByBotRow, error) {
	return nil, nil
}

func (q *sessionUpdateQueries) CountMessagesBySession(_ context.Context, _ pgtype.UUID) (int64, error) {
	return q.messageCount, nil
}

func (q *sessionUpdateQueries) UpdateSessionTypeAndMetadata(_ context.Context, arg sqlc.UpdateSessionTypeAndMetadataParams) (sqlc.BotSession, error) {
	q.updateCalled = true
	q.updateParams = arg
	updated := q.session
	updated.Type = arg.Type
	updated.Metadata = arg.Metadata
	return updated, nil
}

func (q *sessionUpdateQueries) UpdateSessionTitle(_ context.Context, arg sqlc.UpdateSessionTitleParams) (sqlc.BotSession, error) {
	q.titleUpdateCalled = true
	updated := q.session
	updated.Title = arg.Title
	return updated, nil
}

func TestUpdateSessionSwitchesEmptyChatToACPAgent(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "22222222-2222-2222-2222-222222222222"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
		session: sqlc.BotSession{
			ID:       testUUID(sessionID),
			BotID:    testUUID(botID),
			Type:     session.TypeChat,
			Title:    "",
			Metadata: testJSON(map[string]any{}),
		},
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	rec, err := callUpdateSession(handler, botID, sessionID, `{"type":"acp_agent","metadata":{"acp_agent_id":"codex","project_path":"/data/app"}}`)
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !queries.updateCalled {
		t.Fatal("UpdateSessionTypeAndMetadata was not called")
	}
	if queries.updateParams.Type != session.TypeACPAgent {
		t.Fatalf("updated type = %q, want %q", queries.updateParams.Type, session.TypeACPAgent)
	}
	var metadata map[string]any
	if err := json.Unmarshal(queries.updateParams.Metadata, &metadata); err != nil {
		t.Fatalf("metadata json = %v", err)
	}
	if metadata["acp_agent_id"] != "codex" || metadata["project_path"] != "/data/app" {
		t.Fatalf("metadata = %#v, want ACP agent metadata", metadata)
	}
}

func TestUpdateSessionDefaultsACPProjectPath(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "22222222-2222-2222-2222-222222222222"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
		session: sqlc.BotSession{
			ID:       testUUID(sessionID),
			BotID:    testUUID(botID),
			Type:     session.TypeChat,
			Title:    "",
			Metadata: testJSON(map[string]any{}),
		},
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	rec, err := callUpdateSession(handler, botID, sessionID, `{"type":"acp_agent","metadata":{"acp_agent_id":"codex"}}`)
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var metadata map[string]any
	if err := json.Unmarshal(queries.updateParams.Metadata, &metadata); err != nil {
		t.Fatalf("metadata json = %v", err)
	}
	if metadata["project_path"] != session.DefaultACPProjectPath || metadata["acp_project_mode"] != session.DefaultACPProjectMode {
		t.Fatalf("metadata = %#v, want default ACP project", metadata)
	}
}

func TestUpdateSessionDefaultsACPProjectPathBeforeAgentChangeCheck(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "22222222-2222-2222-2222-222222222222"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
		session: sqlc.BotSession{
			ID:    testUUID(sessionID),
			BotID: testUUID(botID),
			Type:  session.TypeACPAgent,
			Title: "",
			Metadata: testJSON(map[string]any{
				"acp_agent_id":     "codex",
				"project_path":     session.DefaultACPProjectPath,
				"acp_project_mode": session.DefaultACPProjectMode,
			}),
		},
		messageCount: 1,
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	rec, err := callUpdateSession(handler, botID, sessionID, `{"type":"acp_agent","metadata":{"acp_agent_id":"codex"}}`)
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var metadata map[string]any
	if err := json.Unmarshal(queries.updateParams.Metadata, &metadata); err != nil {
		t.Fatalf("metadata json = %v", err)
	}
	if metadata["project_path"] != session.DefaultACPProjectPath || metadata["acp_project_mode"] != session.DefaultACPProjectMode {
		t.Fatalf("metadata = %#v, want default ACP project", metadata)
	}
}

func TestUpdateSessionRejectsAgentChangeAfterFirstMessage(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "33333333-3333-3333-3333-333333333333"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
		session: sqlc.BotSession{
			ID:       testUUID(sessionID),
			BotID:    testUUID(botID),
			Type:     session.TypeChat,
			Title:    "",
			Metadata: testJSON(map[string]any{}),
		},
		messageCount: 1,
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	_, err := callUpdateSession(handler, botID, sessionID, `{"type":"acp_agent","metadata":{"acp_agent_id":"codex","project_path":"/data/app"}}`)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("UpdateSession() error = %v, want HTTP 409", err)
	}
	if queries.updateCalled {
		t.Fatal("UpdateSessionTypeAndMetadata should not be called after the session is locked")
	}
}

func TestUpdateSessionRejectsRetagToSubagentForChatUser(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "33333333-3333-3333-3333-333333333333"
	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:              testUUID(sessionID),
			BotID:           testUUID(botID),
			Type:            session.TypeChat,
			Title:           "",
			Metadata:        testJSON(map[string]any{}),
			CreatedByUserID: testUUID(userID),
		},
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("user"),
	)

	_, err := callUpdateSessionAs(handler, botID, sessionID, userID, `{"type":"subagent","metadata":{"agent_id":"direct"}}`)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusForbidden {
		t.Fatalf("UpdateSession() error = %v, want HTTP 403", err)
	}
	if queries.updateCalled {
		t.Fatal("chat user should not be able to retag sessions as subagent")
	}
}

func TestGetSessionAllowsChatUserToReadOwnSubagent(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "33333333-3333-3333-3333-333333333333"
	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:              testUUID(sessionID),
			BotID:           testUUID(botID),
			Type:            session.TypeSubagent,
			Title:           "spawned worker",
			Metadata:        testJSON(map[string]any{"agent_id": "worker"}),
			CreatedByUserID: testUUID(userID),
		},
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("user"),
	)

	rec, err := callGetSession(handler, botID, sessionID, userID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUpdateSessionRejectsSubagentTitleUpdateForChatUser(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "33333333-3333-3333-3333-333333333333"
	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:              testUUID(sessionID),
			BotID:           testUUID(botID),
			Type:            session.TypeSubagent,
			Title:           "spawned worker",
			Metadata:        testJSON(map[string]any{"agent_id": "worker"}),
			CreatedByUserID: testUUID(userID),
		},
	}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("user"),
	)

	_, err := callUpdateSessionAs(handler, botID, sessionID, userID, `{"title":"renamed directly"}`)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusForbidden {
		t.Fatalf("UpdateSession() error = %v, want HTTP 403", err)
	}
	if queries.titleUpdateCalled {
		t.Fatal("chat user should not be able to title-update subagent sessions directly")
	}
}

func TestUpdateSessionAllowsEmptyACPAgentChangeAndClosesRuntime(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "55555555-5555-5555-5555-555555555555"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
				},
			},
		}),
		session: sqlc.BotSession{
			ID:    testUUID(sessionID),
			BotID: testUUID(botID),
			Type:  session.TypeACPAgent,
			Title: "",
			Metadata: testJSON(map[string]any{
				"acp_agent_id": "codex",
				"project_path": "/data/app",
			}),
		},
		messageCount: 0,
	}
	closer := &recordingACPSessionCloser{active: map[string]bool{sessionID: true}}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		closer,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	rec, err := callUpdateSession(handler, botID, sessionID, `{"type":"acp_agent","metadata":{"acp_agent_id":"codex","project_path":"/data/other"}}`)
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !queries.updateCalled {
		t.Fatal("UpdateSessionTypeAndMetadata was not called")
	}
	if len(closer.closed) != 1 || closer.closed[0] != sessionID {
		t.Fatalf("closed ACP sessions = %#v, want [%s]", closer.closed, sessionID)
	}
}

func TestUpdateSessionSwitchesACPAgentToChatClearsMetadataAndClosesRuntime(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "44444444-4444-4444-4444-444444444444"
	queries := &sessionUpdateQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:    testUUID(sessionID),
			BotID: testUUID(botID),
			Type:  session.TypeACPAgent,
			Title: "Codex",
			Metadata: testJSON(map[string]any{
				"acp_agent_id":     "codex",
				"project_path":     "/data/app",
				"acp_project_mode": "project",
				"acp_session_id":   "runtime-1",
				"acp_status":       "active",
			}),
		},
	}
	closer := &recordingACPSessionCloser{}
	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		closer,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	rec, err := callUpdateSession(handler, botID, sessionID, `{"type":"chat"}`)
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if queries.updateParams.Type != session.TypeChat {
		t.Fatalf("updated type = %q, want %q", queries.updateParams.Type, session.TypeChat)
	}
	var metadata map[string]any
	if err := json.Unmarshal(queries.updateParams.Metadata, &metadata); err != nil {
		t.Fatalf("metadata json = %v", err)
	}
	for _, key := range []string{"acp_agent_id", "project_path", "acp_project_mode", "acp_session_id", "acp_status"} {
		if _, ok := metadata[key]; ok {
			t.Fatalf("metadata key %q was not cleared: %#v", key, metadata)
		}
	}
	if len(closer.closed) != 1 || closer.closed[0] != sessionID {
		t.Fatalf("closed ACP sessions = %#v, want [%s]", closer.closed, sessionID)
	}
}

func callUpdateSession(handler *SessionHandler, botID, sessionID, body string) (*httptest.ResponseRecorder, error) {
	return callUpdateSessionAs(handler, botID, sessionID, "user-1", body)
}

func callUpdateSessionAs(handler *SessionHandler, botID, sessionID, userID, body string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/bots/"+botID+"/sessions/"+sessionID, bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, userID)
	ctx.SetPath("/bots/:bot_id/sessions/:session_id")
	ctx.SetParamNames("bot_id", "session_id")
	ctx.SetParamValues(botID, sessionID)
	return rec, handler.UpdateSession(ctx)
}

func callGetSession(handler *SessionHandler, botID, sessionID, userID string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots/"+botID+"/sessions/"+sessionID, nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, userID)
	ctx.SetPath("/bots/:bot_id/sessions/:session_id")
	ctx.SetParamNames("bot_id", "session_id")
	ctx.SetParamValues(botID, sessionID)
	return rec, handler.GetSession(ctx)
}
