package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/session"
)

type sessionDeleteQueries struct {
	dbstore.Queries
	bot              sqlc.GetBotByIDRow
	session          sqlc.BotSession
	softDeleteCalled bool
	softDeleteID     pgtype.UUID
}

func (q *sessionDeleteQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q *sessionDeleteQueries) GetSessionByID(_ context.Context, _ pgtype.UUID) (sqlc.BotSession, error) {
	return q.session, nil
}

func (q *sessionDeleteQueries) SoftDeleteSession(_ context.Context, id pgtype.UUID) error {
	q.softDeleteCalled = true
	q.softDeleteID = id
	return nil
}

type recordingACPSessionCloser struct {
	closed []string
	active map[string]bool
}

func (c *recordingACPSessionCloser) CloseSession(sessionID string) error {
	c.closed = append(c.closed, sessionID)
	return nil
}

func (*recordingACPSessionCloser) BindRuntime(_, _, _, _, _ string) error {
	return nil
}

func (c *recordingACPSessionCloser) IsSessionActive(sessionID string) bool {
	return c.active != nil && c.active[sessionID]
}

func TestDeleteACPAgentSessionClosesRuntimeBeforeSoftDelete(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "22222222-2222-2222-2222-222222222222"
	queries := &sessionDeleteQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:       testUUID(sessionID),
			BotID:    testUUID(botID),
			Type:     session.TypeACPAgent,
			Title:    "Codex",
			Metadata: testJSON(map[string]any{"acp_agent_id": "codex", "project_path": "/data/app"}),
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

	rec, err := callDeleteSession(handler, botID, sessionID)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(closer.closed) != 1 || closer.closed[0] != sessionID {
		t.Fatalf("closed ACP sessions = %#v, want [%s]", closer.closed, sessionID)
	}
	if !queries.softDeleteCalled || queries.softDeleteID != testUUID(sessionID) {
		t.Fatalf("soft delete = %v id=%v, want session %s", queries.softDeleteCalled, queries.softDeleteID, sessionID)
	}
}

func TestDeleteChatSessionDoesNotCloseACPRuntime(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "33333333-3333-3333-3333-333333333333"
	queries := &sessionDeleteQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:       testUUID(sessionID),
			BotID:    testUUID(botID),
			Type:     session.TypeChat,
			Title:    "Chat",
			Metadata: testJSON(map[string]any{}),
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

	if _, err := callDeleteSession(handler, botID, sessionID); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if len(closer.closed) != 0 {
		t.Fatalf("chat session closed ACP runtime: %#v", closer.closed)
	}
	if !queries.softDeleteCalled {
		t.Fatal("chat session was not soft-deleted")
	}
}

func callDeleteSession(handler *SessionHandler, botID, sessionID string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/bots/"+botID+"/sessions/"+sessionID, nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, "user-1")
	ctx.SetPath("/bots/:bot_id/sessions/:session_id")
	ctx.SetParamNames("bot_id", "session_id")
	ctx.SetParamValues(botID, sessionID)
	return rec, handler.DeleteSession(ctx)
}
