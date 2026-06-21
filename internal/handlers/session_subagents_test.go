package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/session"
)

func TestListSubagentsReturnsChildSessions(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	parentSessionID := "22222222-2222-2222-2222-222222222222"
	childSessionID := "33333333-3333-3333-3333-333333333333"
	adminID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	now := time.Now().UTC().Truncate(time.Microsecond)

	queries := &subagentListQueries{
		bot: testBotRow(botID, map[string]any{}),
		parentSession: sqlc.BotSession{
			ID:        testUUID(parentSessionID),
			BotID:     testUUID(botID),
			Type:      session.TypeChat,
			Title:     "Parent",
			Metadata:  testJSON(map[string]any{}),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		subagents: []sqlc.BotSession{
			{
				ID:              testUUID(childSessionID),
				BotID:           testUUID(botID),
				Type:            session.TypeSubagent,
				Title:           "Research task",
				ParentSessionID: testUUID(parentSessionID),
				Metadata:        testJSON(map[string]any{"agent_id": "agent_1"}),
				CreatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
				UpdatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
			},
		},
	}

	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots/"+botID+"/sessions/"+parentSessionID+"/subagents", nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, adminID)
	ctx.SetParamNames("bot_id", "session_id")
	ctx.SetParamValues(botID, parentSessionID)

	if err := handler.ListSubagents(ctx); err != nil {
		t.Fatalf("ListSubagents() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp listSubagentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(resp.Items))
	}
	if resp.Items[0].ID != childSessionID {
		t.Fatalf("item[0].ID = %q, want %q", resp.Items[0].ID, childSessionID)
	}
	if resp.Items[0].Type != session.TypeSubagent {
		t.Fatalf("item[0].Type = %q, want %q", resp.Items[0].Type, session.TypeSubagent)
	}
}

func TestListSubagentsReturnsEmptySlice(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	parentSessionID := "22222222-2222-2222-2222-222222222222"
	adminID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	now := time.Now().UTC().Truncate(time.Microsecond)

	queries := &subagentListQueries{
		bot: testBotRow(botID, map[string]any{}),
		parentSession: sqlc.BotSession{
			ID:        testUUID(parentSessionID),
			BotID:     testUUID(botID),
			Type:      session.TypeChat,
			Title:     "Parent",
			Metadata:  testJSON(map[string]any{}),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		subagents: nil,
	}

	handler := NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries, nil),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots/"+botID+"/sessions/"+parentSessionID+"/subagents", nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, adminID)
	ctx.SetParamNames("bot_id", "session_id")
	ctx.SetParamValues(botID, parentSessionID)

	if err := handler.ListSubagents(ctx); err != nil {
		t.Fatalf("ListSubagents() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp listSubagentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Items == nil {
		t.Fatal("items should be empty slice, not null")
	}
	if len(resp.Items) != 0 {
		t.Fatalf("got %d items, want 0", len(resp.Items))
	}
}

// --- Test doubles ---

type subagentListQueries struct {
	dbstore.Queries
	bot           sqlc.GetBotByIDRow
	parentSession sqlc.BotSession
	subagents     []sqlc.BotSession
}

func (q *subagentListQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q *subagentListQueries) GetSessionByID(_ context.Context, _ pgtype.UUID) (sqlc.BotSession, error) {
	return q.parentSession, nil
}

func (q *subagentListQueries) ListSubagentSessionsByParent(_ context.Context, _ pgtype.UUID) ([]sqlc.BotSession, error) {
	if q.subagents == nil {
		return []sqlc.BotSession{}, nil
	}
	return q.subagents, nil
}

func (*subagentListQueries) ListSessionsByBot(_ context.Context, _ pgtype.UUID) ([]sqlc.ListSessionsByBotRow, error) {
	return nil, nil
}
