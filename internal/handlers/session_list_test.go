package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/session"
)

// sessionListQueries records the arguments passed to the paged list queries
// and returns a configurable canned page so tests can assert filter and cursor
// behavior without a real database.
type sessionListQueries struct {
	dbstore.Queries
	bot                sqlc.GetBotByIDRow
	pagedCall          sqlc.ListSessionsByBotPagedParams
	pagedCallCount     int
	pagedRows          []sqlc.ListSessionsByBotPagedRow
	userPagedCall      sqlc.ListSessionsByBotAndCreatedByUserPagedParams
	userPagedCallCount int
	userPagedRows      []sqlc.ListSessionsByBotAndCreatedByUserPagedRow
}

func (q *sessionListQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q *sessionListQueries) ListSessionsByBotPaged(_ context.Context, arg sqlc.ListSessionsByBotPagedParams) ([]sqlc.ListSessionsByBotPagedRow, error) {
	q.pagedCall = arg
	q.pagedCallCount++
	return q.pagedRows, nil
}

func (q *sessionListQueries) ListSessionsByBotAndCreatedByUserPaged(_ context.Context, arg sqlc.ListSessionsByBotAndCreatedByUserPagedParams) ([]sqlc.ListSessionsByBotAndCreatedByUserPagedRow, error) {
	q.userPagedCall = arg
	q.userPagedCallCount++
	return q.userPagedRows, nil
}

func newListSessionHandler(t *testing.T, queries *sessionListQueries) *SessionHandler {
	t.Helper()
	return NewSessionHandler(
		slog.Default(),
		session.NewService(nil, queries),
		nil,
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)
}

func callListSessions(handler *SessionHandler, botID, rawQuery string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	target := "/bots/" + botID + "/sessions"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	ctx.SetPath("/bots/:bot_id/sessions")
	ctx.SetParamNames("bot_id")
	ctx.SetParamValues(botID)
	return rec, handler.ListSessions(ctx)
}

func decodeListResponse(t *testing.T, rec *httptest.ResponseRecorder) listSessionsResponse {
	t.Helper()
	var resp listSessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestListSessionsDefaultsToUserFacingTypes(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionListQueries{bot: testBotRow(botID, nil)}
	handler := newListSessionHandler(t, queries)

	if _, err := callListSessions(handler, botID, ""); err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if queries.pagedCallCount != 1 {
		t.Fatalf("ListSessionsByBotPaged called %d times, want 1", queries.pagedCallCount)
	}
	got := append([]string(nil), queries.pagedCall.Types...)
	sort.Strings(got)
	want := append([]string(nil), session.UserFacingSessionTypes...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("default types = %v, want %v", got, want)
	}
	if queries.pagedCall.LimitCount != sessionListDefaultLimit {
		t.Fatalf("default limit = %d, want %d", queries.pagedCall.LimitCount, sessionListDefaultLimit)
	}
	if queries.pagedCall.UseCursor {
		t.Fatalf("UseCursor should be false when no cursor was supplied")
	}
}

func TestListSessionsPassesExplicitTypesFilter(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionListQueries{bot: testBotRow(botID, nil)}
	handler := newListSessionHandler(t, queries)

	if _, err := callListSessions(handler, botID, "types=chat,discuss"); err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if got := strings.Join(queries.pagedCall.Types, ","); got != "chat,discuss" {
		t.Fatalf("types filter = %q, want %q", got, "chat,discuss")
	}
}

func TestListSessionsRejectsUnknownType(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionListQueries{bot: testBotRow(botID, nil)}
	handler := newListSessionHandler(t, queries)

	_, err := callListSessions(handler, botID, "types=chat,bogus")
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("ListSessions() error = %v, want HTTP 400", err)
	}
	if queries.pagedCallCount != 0 {
		t.Fatalf("query should not run when types validation fails")
	}
}

func TestListSessionsClampsLimit(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionListQueries{bot: testBotRow(botID, nil)}
	handler := newListSessionHandler(t, queries)

	if _, err := callListSessions(handler, botID, "limit=500"); err == nil {
		t.Fatalf("expected limit out of range to return an error")
	}
	if _, err := callListSessions(handler, botID, "limit=0"); err == nil {
		t.Fatalf("expected limit=0 to return an error")
	}
	if _, err := callListSessions(handler, botID, "limit=not-a-number"); err == nil {
		t.Fatalf("expected non-integer limit to return an error")
	}
	if _, err := callListSessions(handler, botID, "limit=10"); err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if queries.pagedCall.LimitCount != 10 {
		t.Fatalf("limit = %d, want 10", queries.pagedCall.LimitCount)
	}
}

func TestListSessionsCursorRoundTrip(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	rowUpdated := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	rowID := "33333333-3333-3333-3333-333333333333"
	queries := &sessionListQueries{
		bot: testBotRow(botID, nil),
		pagedRows: []sqlc.ListSessionsByBotPagedRow{
			pagedRow(rowID, rowUpdated),
		},
	}
	handler := newListSessionHandler(t, queries)

	// With limit=1 and 1 returned row, next_cursor must point at the last row.
	rec, err := callListSessions(handler, botID, "limit=1")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	resp := decodeListResponse(t, rec)
	if resp.NextCursor == "" {
		t.Fatalf("expected next_cursor when page is full")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(resp.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[1] != rowID {
		t.Fatalf("cursor payload = %q, want trailing id %s", string(decoded), rowID)
	}

	// Round-trip the cursor back in; the handler must decode it and forward
	// the parsed values to the query layer.
	queries.pagedCall = sqlc.ListSessionsByBotPagedParams{}
	if _, err := callListSessions(handler, botID, "limit=1&cursor="+url.QueryEscape(resp.NextCursor)); err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if !queries.pagedCall.UseCursor {
		t.Fatalf("UseCursor should be true when a cursor was supplied")
	}
	if !queries.pagedCall.CursorUpdatedAt.Time.Equal(rowUpdated) {
		t.Fatalf("CursorUpdatedAt = %v, want %v", queries.pagedCall.CursorUpdatedAt.Time, rowUpdated)
	}
	if queries.pagedCall.CursorID.String() != rowID {
		t.Fatalf("CursorID = %s, want %s", queries.pagedCall.CursorID.String(), rowID)
	}
}

func TestListSessionsOmitsNextCursorOnShortPage(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionListQueries{
		bot: testBotRow(botID, nil),
		pagedRows: []sqlc.ListSessionsByBotPagedRow{
			pagedRow("33333333-3333-3333-3333-333333333333", time.Now()),
		},
	}
	handler := newListSessionHandler(t, queries)

	rec, err := callListSessions(handler, botID, "limit=10")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	resp := decodeListResponse(t, rec)
	if resp.NextCursor != "" {
		t.Fatalf("next_cursor = %q, want empty when page is not full", resp.NextCursor)
	}
}

func TestListSessionsRejectsMalformedCursor(t *testing.T) {
	botID := "11111111-1111-1111-1111-111111111111"
	queries := &sessionListQueries{bot: testBotRow(botID, nil)}
	handler := newListSessionHandler(t, queries)

	_, err := callListSessions(handler, botID, "cursor=not%20base64%21")
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("ListSessions() error = %v, want HTTP 400", err)
	}
}

func pagedRow(id string, updatedAt time.Time) sqlc.ListSessionsByBotPagedRow {
	return sqlc.ListSessionsByBotPagedRow{
		ID:        testUUID(id),
		BotID:     testUUID("11111111-1111-1111-1111-111111111111"),
		Type:      session.TypeChat,
		Title:     "test session",
		Metadata:  []byte(`{}`),
		CreatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
	}
}
