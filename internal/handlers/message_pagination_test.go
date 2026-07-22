package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
)

// stubMessageService mirrors the production ordering contract of the message
// service: ListLatestBySession returns DESC (newest-first, as the DB rows come
// back), while ListBeforeBySession returns ASC (oldest-first, because its
// converter reverses the DESC rows). Tests exercise the handler against this
// real wire shape rather than a hand-constructed slice. It embeds the interface
// so the other (unused) methods satisfy it without boilerplate; calling them
// panics.
type stubMessageService struct {
	messagepkg.Service
	bySession            map[string][]messagepkg.Message
	visibleTurnSessionID string
	visibleTurnID        string
	visibleTurnResult    []messagepkg.Message
	beforeMessageCalls   int
}

func (s *stubMessageService) latest(sid string, limit int) []messagepkg.Message {
	all := s.bySession[sid]
	desc := make([]messagepkg.Message, len(all))
	copy(desc, all)
	sort.Slice(desc, func(i, j int) bool { return desc[i].CreatedAt.After(desc[j].CreatedAt) })
	if len(desc) > limit {
		desc = desc[:limit]
	}
	return desc
}

func (s *stubMessageService) before(sid string, t time.Time, limit int) []messagepkg.Message {
	var older []messagepkg.Message
	for _, m := range s.bySession[sid] {
		if m.CreatedAt.Before(t) {
			older = append(older, m)
		}
	}
	sort.Slice(older, func(i, j int) bool { return older[i].CreatedAt.After(older[j].CreatedAt) })
	if len(older) > limit {
		older = older[:limit]
	}
	reverseMessages(older)
	return older
}

func (s *stubMessageService) ListLatestBySession(_ context.Context, sid string, limit int32) ([]messagepkg.Message, error) {
	return s.latest(sid, int(limit)), nil
}

func (s *stubMessageService) ListBeforeBySession(_ context.Context, sid string, before time.Time, limit int32) ([]messagepkg.Message, error) {
	return s.before(sid, before, int(limit)), nil
}

func (s *stubMessageService) ListBeforeMessageBySession(_ context.Context, sid string, beforeMessageID string, limit int32) ([]messagepkg.Message, error) {
	s.beforeMessageCalls++
	for _, m := range s.bySession[sid] {
		if m.ID == beforeMessageID {
			return s.before(sid, m.CreatedAt, int(limit)), nil
		}
	}
	return nil, nil
}

func (s *stubMessageService) ListVisibleMessagesByTurnIDBySession(_ context.Context, sessionID string, turnID string) ([]messagepkg.Message, error) {
	s.visibleTurnSessionID = sessionID
	s.visibleTurnID = turnID
	return s.visibleTurnResult, nil
}

type messagePaginationQueries struct {
	dbstore.Queries
	bot     sqlc.GetBotByIDRow
	session sqlc.BotSession
}

func (q *messagePaginationQueries) GetBotByID(_ context.Context, _ pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q *messagePaginationQueries) GetSessionByID(_ context.Context, _ pgtype.UUID) (sqlc.BotSession, error) {
	return q.session, nil
}

func TestListMessagesTurnIDReturnsCompleteUIExactTurn(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		turnID    = "33333333-3333-3333-3333-333333333333"
	)
	queries := &messagePaginationQueries{
		bot: testBotRow(botID, map[string]any{}),
		session: sqlc.BotSession{
			ID:          testUUID(sessionID),
			BotID:       testUUID(botID),
			ChannelType: pgtype.Text{String: "local", Valid: true},
		},
	}
	messageService := &stubMessageService{visibleTurnResult: []messagepkg.Message{
		{
			ID: "44444444-4444-4444-4444-444444444441", BotID: botID, SessionID: sessionID,
			Role: "user", Content: []byte(`{"role":"user","content":"hello"}`), DisplayContent: "hello",
			TurnID: turnID, TurnPosition: 7, TurnMessageSeq: 1,
		},
		{
			ID: "44444444-4444-4444-4444-444444444442", BotID: botID, SessionID: sessionID,
			Role: "assistant", Content: []byte(`{"role":"assistant","content":"done"}`),
			TurnID: turnID, TurnPosition: 7, TurnMessageSeq: 2,
		},
	}}
	handler := NewMessageHandler(
		slog.Default(),
		nil,
		messageService,
		session.NewService(nil, queries, nil),
		bots.NewService(nil, queries),
		newTestAdminAccountService("admin"),
	)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots/"+botID+"/messages?session_id="+sessionID+"&turn_id="+turnID+"&format=ui&limit=1", nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	ctx.SetPath("/bots/:bot_id/messages")
	ctx.SetParamNames("bot_id")
	ctx.SetParamValues(botID)

	if err := handler.ListMessages(ctx); err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if messageService.visibleTurnSessionID != sessionID || messageService.visibleTurnID != turnID {
		t.Fatalf("exact turn args = %q/%q, want %q/%q", messageService.visibleTurnSessionID, messageService.visibleTurnID, sessionID, turnID)
	}
	if messageService.beforeMessageCalls != 0 {
		t.Fatalf("exact turn UI query extended into prior history %d times", messageService.beforeMessageCalls)
	}
	var payload struct {
		Items []conversation.UITurn `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 2 || payload.Items[0].Role != "user" || payload.Items[1].Role != "assistant" {
		t.Fatalf("exact turn UI items = %#v, want complete user/assistant turn", payload.Items)
	}
}

func TestListMessagesRejectsInvalidTurnID(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots/11111111-1111-1111-1111-111111111111/messages?session_id=22222222-2222-2222-2222-222222222222&turn_id=not-a-uuid", nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	ctx.SetParamNames("bot_id")
	ctx.SetParamValues("11111111-1111-1111-1111-111111111111")

	err := (&MessageHandler{messageService: &stubMessageService{}}).ListMessages(ctx)
	assertHTTPErrorCode(t, err, http.StatusBadRequest)
}

func TestListMessagesRejectsConflictingTurnIDCursor(t *testing.T) {
	t.Parallel()

	const turnID = "33333333-3333-3333-3333-333333333333"
	tests := []string{
		"before_message_id=44444444-4444-4444-4444-444444444444",
		"before=2026-07-16T00:00:00Z",
	}
	for _, query := range tests {
		query := query
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/bots/11111111-1111-1111-1111-111111111111/messages?session_id=22222222-2222-2222-2222-222222222222&turn_id="+turnID+"&"+query, nil)
			rec := httptest.NewRecorder()
			ctx := testAuthContext(e, req, rec, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			ctx.SetParamNames("bot_id")
			ctx.SetParamValues("11111111-1111-1111-1111-111111111111")

			err := (&MessageHandler{messageService: &stubMessageService{}}).ListMessages(ctx)
			assertHTTPErrorCode(t, err, http.StatusBadRequest)
		})
	}
}

func assertHTTPErrorCode(t *testing.T, err error, want int) {
	t.Helper()
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T %v, want *echo.HTTPError", err, err)
	}
	if httpErr.Code != want {
		t.Fatalf("HTTP status = %d, want %d", httpErr.Code, want)
	}
}

func msg(role string, t time.Time) messagepkg.Message {
	return messagepkg.Message{ID: role + "-" + t.Format("150405.000000000"), Role: role, CreatedAt: t, Content: []byte(`{}`)}
}

// userMsg builds a visible user message — IsUITurnBoundary requires non-empty
// text (DisplayContent is the first source it checks), otherwise the row is
// treated as an invisible user ping and NOT a boundary, which would defeat the
// test.
func userMsg(t time.Time, text string) messagepkg.Message {
	return messagepkg.Message{ID: "user-" + t.Format("150405.000000000"), Role: "user", CreatedAt: t, Content: []byte(`{}`), DisplayContent: text}
}

// TestExtendToUITurnHead_PreservesMonotonicOrder is the regression test for the
// before-page double-reverse bug: ListBeforeBySession already returns
// oldest-first (ASC), so extendToUITurnHead must prepend each fetched older
// batch as-is to keep the combined slice monotonic - the ordering
// ConvertMessagesToUITurns depends on. The bug reversed the already-ASC batch a
// second time, producing a scrambled, non-monotonic slice that split one turn
// into several and duplicated turns.
func TestExtendToUITurnHead_PreservesMonotonicOrder(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	// A session whose latest page (limit 30) lands mid-turn: a user boundary,
	// then 40 assistant/tool rows forming one turn. Ask for the latest 30.
	const sessionID = "s1"
	var all []messagepkg.Message
	all = append(all, userMsg(base, "hello")) // turn boundary (oldest)
	for i := 1; i <= 40; i++ {
		all = append(all, msg("assistant", base.Add(time.Duration(i)*time.Second)))
		all = append(all, msg("tool", base.Add(time.Duration(i)*time.Second+time.Millisecond)))
	}
	svc := &stubMessageService{bySession: map[string][]messagepkg.Message{sessionID: all}}
	h := &MessageHandler{messageService: svc, logger: slog.Default()}

	latest := svc.latest(sessionID, 30)
	reverseMessages(latest) // mirrors the handler's latest-page branch
	before := len(latest)
	got := h.extendToUITurnHead(context.Background(), sessionID, latest, 30)

	if len(got) <= before {
		t.Fatalf("extendToUITurnHead did not pull back the turn head: got %d, had %d", len(got), before)
	}
	if got[0].Role != "user" {
		t.Fatalf("expected the pulled-back head to be the user boundary, got role %q", got[0].Role)
	}
	for i := 1; i < len(got); i++ {
		if got[i].CreatedAt.Before(got[i-1].CreatedAt) {
			t.Fatalf("non-monotonic order at index %d: older batch prepended without reversing (DESC bug)", i)
		}
	}
}

// TestExtendToUITurnHead_StopsAtBoundary asserts the loop does not over-pull
// once a real turn boundary is at messages[0]. The session has 1 user + 5
// assistant; the latest page (limit 5) returns the 5 newest (all assistant),
// so extendToUITurnHead pulls exactly the one older user row and stops — it
// must NOT keep pulling past the boundary (the double-reverse bug mis-ordered
// the batch so messages[0] was no longer the true oldest row).
func TestExtendToUITurnHead_StopsAtBoundary(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	const sessionID = "s2"
	var all []messagepkg.Message
	all = append(all, userMsg(base, "hello"))
	for i := 1; i <= 5; i++ {
		all = append(all, msg("assistant", base.Add(time.Duration(i)*time.Second)))
	}
	svc := &stubMessageService{bySession: map[string][]messagepkg.Message{sessionID: all}}
	h := &MessageHandler{messageService: svc, logger: slog.Default()}

	latest := svc.latest(sessionID, 5) // 5 newest = all assistant, no boundary
	reverseMessages(latest)
	got := h.extendToUITurnHead(context.Background(), sessionID, latest, 5)
	// Must pull back exactly the one user boundary and stop — 6 total, not more.
	if len(got) != 6 {
		t.Fatalf("expected exactly the user boundary + 5 assistant = 6, got %d (over-pulled?)", len(got))
	}
	if got[0].Role != "user" {
		t.Fatalf("expected head to be the user boundary, got role %q", got[0].Role)
	}
}

func TestExtendToUITurnHead_CapsPathologicalTurn(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	const sessionID = "s3"
	var all []messagepkg.Message
	all = append(all, userMsg(base, "hello"))
	for i := 1; i <= 500; i++ {
		all = append(all, msg("assistant", base.Add(time.Duration(i)*time.Second)))
	}
	svc := &stubMessageService{bySession: map[string][]messagepkg.Message{sessionID: all}}
	h := &MessageHandler{messageService: svc, logger: slog.Default()}

	latest := svc.latest(sessionID, 30)
	reverseMessages(latest)
	got := h.extendToUITurnHead(context.Background(), sessionID, latest, 30)

	if len(got) != uiTurnHeadExtensionLimit(30, 30) {
		t.Fatalf("expected extension to stop at cap, got %d", len(got))
	}
	if got[0].Role == "user" {
		t.Fatalf("expected pathological turn to remain capped before reaching the user boundary")
	}
}
