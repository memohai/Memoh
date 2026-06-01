package handlers

import (
	"bytes"
	"context"
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
