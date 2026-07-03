package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/session"
)

// TestMessagesEventsRouteIsRemoved guards the route contract: the bot-wide
// `/messages/events` endpoint was deleted as part of the SSE split. A future
// re-introduction by accident — say, by reverting the route registration —
// would make the legacy frontend re-attach silently.
func TestMessagesEventsRouteIsRemoved(t *testing.T) {
	t.Parallel()

	e := echo.New()
	(&MessageHandler{}).Register(e)

	req := httptest.NewRequest(http.MethodGet, "/bots/bot-1/messages/events", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /bots/bot-1/messages/events status = %d, want 404", rec.Code)
	}
}

// TestListMessagesRequiresSessionID pins the contract that ListMessages
// rejects requests without a session_id. The pre-split endpoint accepted
// bot-wide listings; preserving the 400 keeps clients from falling back to
// that shape silently.
func TestListMessagesRequiresSessionID(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots/bot-1/messages", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer test")
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	ctx.SetPath("/bots/:bot_id/messages")
	ctx.SetParamNames("bot_id")
	ctx.SetParamValues("11111111-1111-1111-1111-111111111111")

	err := h.ListMessages(ctx)
	if err == nil {
		t.Fatalf("ListMessages() err = nil, want HTTP 400")
	}
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusBadRequest {
		t.Fatalf("HTTPError.Code = %d, want 400", httpErr.Code)
	}
}

func TestSessionHeadTurnIDForVariantCapableSession(t *testing.T) {
	t.Parallel()

	if got := sessionHeadTurnIDForVariantCapableSession(session.Session{Type: session.TypeChat}, " turn-c "); got != "turn-c" {
		t.Fatalf("chat head = %q, want turn-c", got)
	}
	for _, typ := range []string{session.TypeDiscuss, session.TypeACPAgent, session.TypeHeartbeat, session.TypeSchedule, session.TypeSubagent} {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			if got := sessionHeadTurnIDForVariantCapableSession(session.Session{Type: typ}, "turn-c"); got != "" {
				t.Fatalf("non-chat head = %q, want empty", got)
			}
		})
	}
}

// TestSubscribeBeforeBacklogDedupsLiveMessage exercises the subscribe-before-
// backlog seam invariant: a message persisted between Subscribe and the
// backlog read shows up in BOTH the backlog query result and the live
// subscriber channel. The handler relies on the backlog ID set to drop the
// duplicate so the wire carries it exactly once.
//
// We test the dedup logic in isolation here (Hub + dedup map). A full HTTP
// integration test would need to fake out bots/accounts/session services for
// authorization; the dedup is self-contained and worth pinning directly.
func TestSubscribeBeforeBacklogDedupsLiveMessage(t *testing.T) {
	t.Parallel()

	hub := messageevent.NewHub()
	botID := "11111111-1111-1111-1111-111111111111"
	sessionID := "22222222-2222-2222-2222-222222222222"

	// Subscribe first — this is what the handler does before reading backlog.
	sub, cancel := hub.Subscribe(botID, 8)
	defer cancel()

	// Persistence happens; both the backlog query and the live channel will
	// see this message. We model the backlog by listing its ID first.
	msg := messagepkg.Message{ID: "m1", BotID: botID, SessionID: sessionID, CreatedAt: time.Now()}
	backlog := []messagepkg.Message{msg}

	// AFTER subscribe and AFTER the backlog read, the hub broadcasts the same
	// message to live subscribers. Without dedup, the client sees m1 twice.
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	hub.Publish(messageevent.Event{
		Type:  messageevent.EventTypeMessageCreated,
		BotID: botID,
		Data:  data,
	})

	// Build the dedup set the same way the handler does.
	backlogIDs := make(map[string]struct{}, len(backlog))
	delivered := map[string]int{}
	for _, b := range backlog {
		backlogIDs[b.ID] = struct{}{}
		delivered[b.ID]++
	}

	// Drain the live channel and apply the same filter the handler does.
	select {
	case ev := <-sub.Events:
		if ev.Type != messageevent.EventTypeMessageCreated {
			t.Fatalf("event type = %q", ev.Type)
		}
		var live messagepkg.Message
		if err := json.Unmarshal(ev.Data, &live); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if live.SessionID != sessionID {
			t.Fatalf("session filter check missed: %q", live.SessionID)
		}
		if _, dup := backlogIDs[live.ID]; dup {
			// Correctly skipped. This is the on-wire path.
		} else {
			delivered[live.ID]++
		}
	case <-time.After(time.Second):
		t.Fatal("no live event received")
	}

	// The seam works iff the message is delivered EXACTLY ONCE on the wire.
	if delivered["m1"] != 1 {
		t.Fatalf("m1 delivered %d times, want exactly 1", delivered["m1"])
	}
}

func TestStreamBacklogUsesSelectedHeadTranscriptPage(t *testing.T) {
	t.Parallel()

	at := time.Unix(1700000000, 0).UTC()
	svc := &streamBacklogMessageService{
		messages: []messagepkg.Message{
			{ID: "m-b", SessionID: "session-1", TurnID: "turn-b", CreatedAt: at.Add(time.Second)},
			{ID: "m-a", SessionID: "session-1", TurnID: "turn-a", CreatedAt: at},
		},
	}
	h := &MessageHandler{messageService: svc}

	messages, err := h.latestSessionMessagesForStreamBacklog(context.Background(), "session-1", "turn-b", false, 50)
	if err != nil {
		t.Fatalf("latestSessionMessagesForStreamBacklog() error = %v", err)
	}
	got := make([]string, 0, len(messages))
	for _, message := range messages {
		got = append(got, message.ID)
	}
	want := []string{"m-a", "m-b"}
	if len(got) != len(want) {
		t.Fatalf("messages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("messages = %v, want %v", got, want)
		}
	}
	if svc.headTurnID != "turn-b" {
		t.Fatalf("selected head = %q, want turn-b", svc.headTurnID)
	}
}

type streamBacklogMessageService struct {
	messagepkg.Service
	headTurnID string
	messages   []messagepkg.Message
}

func (s *streamBacklogMessageService) ListLatestBySessionHead(_ context.Context, _ string, headTurnID string, _ int32) ([]messagepkg.Message, error) {
	s.headTurnID = headTurnID
	return append([]messagepkg.Message(nil), s.messages...), nil
}

func (*streamBacklogMessageService) ListBeforeBySessionHead(context.Context, string, string, time.Time, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

// TestAcceptLiveTurnAllowsLinearExtension pins the live-filter contract: an
// unseen live message is visible only when the current live head sits on the
// message turn's ancestor path. Accepted descendants become the new live head;
// sibling variants stay hidden.
//
// Fixture paths (child -> root): a <- b <- d <- e, plus sibling c <- a.
func TestAcceptLiveTurnAllowsLinearExtension(t *testing.T) {
	t.Parallel()

	svc := &turnAncestorMessageService{matches: map[ancestorCheck]bool{
		{turnID: "turn-d", ancestorTurnID: "turn-b"}: true,
		{turnID: "turn-e", ancestorTurnID: "turn-d"}: true,
		{turnID: "turn-c", ancestorTurnID: "turn-e"}: false,
	}}
	h := &MessageHandler{messageService: svc}

	visible, liveHead, err := h.acceptLiveTurn(context.Background(), "turn-b", "turn-d")
	if err != nil {
		t.Fatalf("acceptLiveTurn(turn-d) error = %v", err)
	}
	if !visible || liveHead != "turn-d" {
		t.Fatalf("turn-d visible/liveHead = %v/%q, want true/turn-d", visible, liveHead)
	}

	visible, liveHead, err = h.acceptLiveTurn(context.Background(), liveHead, "turn-e")
	if err != nil {
		t.Fatalf("acceptLiveTurn(turn-e) error = %v", err)
	}
	if !visible || liveHead != "turn-e" {
		t.Fatalf("turn-e visible/liveHead = %v/%q, want true/turn-e", visible, liveHead)
	}

	visible, liveHead, err = h.acceptLiveTurn(context.Background(), liveHead, "turn-c")
	if err != nil {
		t.Fatalf("acceptLiveTurn(turn-c) error = %v", err)
	}
	if visible || liveHead != "turn-e" {
		t.Fatalf("turn-c visible/liveHead = %v/%q, want false/turn-e", visible, liveHead)
	}
}

func TestAcceptLiveTurnCurrentHeadPassesWithoutQuery(t *testing.T) {
	t.Parallel()

	svc := &turnAncestorMessageService{}
	h := &MessageHandler{messageService: svc}
	visible, liveHead, err := h.acceptLiveTurn(context.Background(), "turn-b", "turn-b")
	if err != nil {
		t.Fatalf("acceptLiveTurn() error = %v", err)
	}
	if !visible || liveHead != "turn-b" {
		t.Fatalf("visible/liveHead = %v/%q, want true/turn-b", visible, liveHead)
	}
	if len(svc.calls) != 0 {
		t.Fatalf("ancestor checks = %v, want none", svc.calls)
	}
}

// TestAcceptLiveTurnFallbackWithoutCheckerStaysPermissive covers non-DB
// fallback services in tests or degraded wiring: without the optimized checker,
// keep live delivery permissive instead of silently dropping selected-view
// events.
func TestAcceptLiveTurnFallbackWithoutCheckerStaysPermissive(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{messageService: &streamBacklogMessageService{}}
	visible, liveHead, err := h.acceptLiveTurn(context.Background(), "turn-b", "turn-x")
	if err != nil {
		t.Fatalf("acceptLiveTurn() error = %v", err)
	}
	if !visible || liveHead != "turn-x" {
		t.Fatalf("visible/liveHead = %v/%q, want true/turn-x", visible, liveHead)
	}
}

// TestAcceptLiveTurnLegacyAllVisible covers sessions without a selected path:
// an empty live head means "no branch filtering", so every live message passes.
func TestAcceptLiveTurnLegacyAllVisible(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{messageService: &turnAncestorMessageService{}}
	visible, liveHead, err := h.acceptLiveTurn(context.Background(), "", "turn-x")
	if err != nil {
		t.Fatalf("acceptLiveTurn() error = %v", err)
	}
	if !visible || liveHead != "" {
		t.Fatalf("visible/liveHead = %v/%q, want true/empty", visible, liveHead)
	}
}

// TestAcceptLiveTurnRejectsBlankTurn pins that a live message without a turn id
// stays hidden once a selected path exists.
func TestAcceptLiveTurnRejectsBlankTurn(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{messageService: &turnAncestorMessageService{}}
	visible, liveHead, err := h.acceptLiveTurn(context.Background(), "turn-a", "  ")
	if err != nil {
		t.Fatalf("acceptLiveTurn() error = %v", err)
	}
	if visible || liveHead != "turn-a" {
		t.Fatalf("visible/liveHead = %v/%q, want false/turn-a", visible, liveHead)
	}
}

// TestResolveSessionViewHeadPassesThroughRealHead verifies the cheap path: a
// requested turn that IS an active head short-circuits through the heads
// table check and never runs the descendant recursion.
func TestResolveSessionViewHeadPassesThroughRealHead(t *testing.T) {
	t.Parallel()

	svc := &viewHeadMessageService{isHead: true}
	h := &MessageHandler{messageService: svc}
	got, err := h.resolveSessionViewHead(context.Background(), "session-1", "turn-b")
	if err != nil {
		t.Fatalf("resolveSessionViewHead() error = %v", err)
	}
	if got != "turn-b" {
		t.Fatalf("resolved head = %q, want turn-b", got)
	}
	if svc.validatorCalls != 1 {
		t.Fatalf("validator calls = %d, want 1", svc.validatorCalls)
	}
	if svc.resolverCalls != 0 {
		t.Fatalf("resolver calls = %d, want 0", svc.resolverCalls)
	}
}

// TestResolveSessionViewHeadResolvesNonHeadTurn verifies variant switching:
// a non-head turn resolves to the head whose path contains it.
func TestResolveSessionViewHeadResolvesNonHeadTurn(t *testing.T) {
	t.Parallel()

	svc := &viewHeadMessageService{isHead: false, resolved: "turn-e"}
	h := &MessageHandler{messageService: svc}
	got, err := h.resolveSessionViewHead(context.Background(), "session-1", "turn-b")
	if err != nil {
		t.Fatalf("resolveSessionViewHead() error = %v", err)
	}
	if got != "turn-e" {
		t.Fatalf("resolved head = %q, want turn-e", got)
	}
	if svc.resolverCalls != 1 {
		t.Fatalf("resolver calls = %d, want 1", svc.resolverCalls)
	}
}

// TestResolveSessionViewHeadEmptyRequestKeepsDefaultView verifies "" means
// "use the session default view" and issues no lookups.
func TestResolveSessionViewHeadEmptyRequestKeepsDefaultView(t *testing.T) {
	t.Parallel()

	svc := &viewHeadMessageService{}
	h := &MessageHandler{messageService: svc}
	got, err := h.resolveSessionViewHead(context.Background(), "session-1", "  ")
	if err != nil {
		t.Fatalf("resolveSessionViewHead() error = %v", err)
	}
	if got != "" {
		t.Fatalf("resolved head = %q, want empty", got)
	}
	if svc.validatorCalls != 0 || svc.resolverCalls != 0 {
		t.Fatalf("lookups = %d/%d, want 0/0", svc.validatorCalls, svc.resolverCalls)
	}
}

// TestResolveSessionViewHeadUnresolvedFallsBackEmpty covers stale pins: a
// turn no active head contains resolves to "" so callers fall back to the
// session default head (the 409 stale-head contract is gone).
func TestResolveSessionViewHeadUnresolvedFallsBackEmpty(t *testing.T) {
	t.Parallel()

	svc := &viewHeadMessageService{isHead: false, resolved: ""}
	h := &MessageHandler{messageService: svc}
	got, err := h.resolveSessionViewHead(context.Background(), "session-1", "turn-gone")
	if err != nil {
		t.Fatalf("resolveSessionViewHead() error = %v", err)
	}
	if got != "" {
		t.Fatalf("resolved head = %q, want empty", got)
	}
	if svc.resolverCalls != 1 {
		t.Fatalf("resolver calls = %d, want 1", svc.resolverCalls)
	}
}

type ancestorCheck struct {
	turnID         string
	ancestorTurnID string
}

type turnAncestorMessageService struct {
	messagepkg.Service
	matches map[ancestorCheck]bool
	calls   []ancestorCheck
}

func (s *turnAncestorMessageService) IsSessionTurnAncestor(_ context.Context, turnID string, ancestorTurnID string) (bool, error) {
	check := ancestorCheck{turnID: turnID, ancestorTurnID: ancestorTurnID}
	s.calls = append(s.calls, check)
	return s.matches[check], nil
}

type viewHeadMessageService struct {
	messagepkg.Service
	isHead         bool
	resolved       string
	validatorCalls int
	resolverCalls  int
}

func (s *viewHeadMessageService) IsSessionTurnHead(context.Context, string, string) (bool, error) {
	s.validatorCalls++
	return s.isHead, nil
}

func (s *viewHeadMessageService) ResolveSessionTurnHead(context.Context, string, string) (string, error) {
	s.resolverCalls++
	return s.resolved, nil
}

// TestStreamSessionMessageEventsRequiresSessionPath confirms the per-session
// stream rejects requests missing the session_id path parameter — ie, a
// client that builds the URL incorrectly gets a 400 instead of an opaque
// streaming response.
func TestStreamSessionMessageEventsRequiresSessionPath(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots/bot-1/sessions//messages/events", nil)
	rec := httptest.NewRecorder()
	ctx := testAuthContext(e, req, rec, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	ctx.SetPath("/bots/:bot_id/sessions/:session_id/messages/events")
	ctx.SetParamNames("bot_id", "session_id")
	ctx.SetParamValues("11111111-1111-1111-1111-111111111111", "")

	err := h.StreamSessionMessageEvents(ctx)
	if err == nil {
		t.Fatalf("StreamSessionMessageEvents() err = nil, want HTTP 400")
	}
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusBadRequest {
		t.Fatalf("HTTPError.Code = %d, want 400", httpErr.Code)
	}
}
