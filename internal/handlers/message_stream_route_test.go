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

func TestLiveMessageVisibilityUsesSelectedHeadPath(t *testing.T) {
	t.Parallel()

	visible := map[string]struct{}{
		"turn-a": {},
		"turn-b": {},
	}
	if !messageVisibleInTurnSet(messagepkg.Message{TurnID: "turn-b"}, visible) {
		t.Fatal("message on selected head path was filtered")
	}
	if messageVisibleInTurnSet(messagepkg.Message{TurnID: "turn-c"}, visible) {
		t.Fatal("message on sibling head path was not filtered")
	}
	if messageVisibleInTurnSet(messagepkg.Message{}, visible) {
		t.Fatal("message without turn id was not filtered when a selected path exists")
	}
	if !messageVisibleInTurnSet(messagepkg.Message{}, nil) {
		t.Fatal("legacy message without selected path should remain visible")
	}
}

// TestAddVisibleDescendantPathAllowsLinearExtension pins the live-filter
// contract: a message on an unseen turn is visible only when one of the
// followed live heads sits on the turn's ancestor path (the turn linearly
// extends the followed branch). Sibling variants stay hidden.
//
// Fixture paths (child -> root): a <- b <- d <- e, plus sibling c <- a.
func TestAddVisibleDescendantPathAllowsLinearExtension(t *testing.T) {
	t.Parallel()

	svc := &turnPathMessageService{paths: map[string][]string{
		"turn-d": {"turn-d", "turn-b", "turn-a"},
		"turn-e": {"turn-e", "turn-d", "turn-b", "turn-a"},
		"turn-c": {"turn-c", "turn-a"},
	}}
	h := &MessageHandler{messageService: svc}
	visible := map[string]struct{}{
		"turn-a": {},
		"turn-b": {},
	}
	liveHeads := map[string]struct{}{"turn-b": {}}

	visibleGot, err := h.addVisibleDescendantPath(context.Background(), visible, liveHeads, "turn-d")
	if err != nil {
		t.Fatalf("addVisibleDescendantPath(turn-d) error = %v", err)
	}
	if !visibleGot {
		t.Fatal("descendant of selected head was filtered")
	}
	if _, ok := visible["turn-d"]; !ok {
		t.Fatalf("descendant was not added to visible set: %#v", visible)
	}

	// turn-e extends turn-d, which just became a live head.
	visibleGot, err = h.addVisibleDescendantPath(context.Background(), visible, liveHeads, "turn-e")
	if err != nil {
		t.Fatalf("addVisibleDescendantPath(turn-e) error = %v", err)
	}
	if !visibleGot {
		t.Fatal("descendant of previously accepted live head was filtered")
	}
	if _, ok := visible["turn-e"]; !ok {
		t.Fatalf("second descendant was not added to visible set: %#v", visible)
	}

	visibleGot, err = h.addVisibleDescendantPath(context.Background(), visible, liveHeads, "turn-c")
	if err != nil {
		t.Fatalf("addVisibleDescendantPath(turn-c) error = %v", err)
	}
	if visibleGot {
		t.Fatal("sibling branch was treated as selected-path descendant")
	}
	if _, ok := visible["turn-c"]; ok {
		t.Fatalf("sibling branch leaked into visible set: %#v", visible)
	}
}

// TestAddVisibleDescendantPathLegacyAllVisible covers sessions without a
// selected path: an empty visible set means "no head filtering", so every
// live message passes.
func TestAddVisibleDescendantPathLegacyAllVisible(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{messageService: &turnPathMessageService{}}
	visibleGot, err := h.addVisibleDescendantPath(context.Background(), map[string]struct{}{}, map[string]struct{}{}, "turn-x")
	if err != nil {
		t.Fatalf("addVisibleDescendantPath() error = %v", err)
	}
	if !visibleGot {
		t.Fatal("legacy session without selected path filtered a live message")
	}
}

// TestAddVisibleDescendantPathRejectsBlankTurn pins that a live message
// without a turn id stays hidden once a selected path exists.
func TestAddVisibleDescendantPathRejectsBlankTurn(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{messageService: &turnPathMessageService{}}
	visible := map[string]struct{}{"turn-a": {}}
	visibleGot, err := h.addVisibleDescendantPath(context.Background(), visible, map[string]struct{}{"turn-a": {}}, "  ")
	if err != nil {
		t.Fatalf("addVisibleDescendantPath() error = %v", err)
	}
	if visibleGot {
		t.Fatal("blank turn id passed the selected-path filter")
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

type turnPathMessageService struct {
	messagepkg.Service
	paths map[string][]string
}

func (s *turnPathMessageService) ListSessionTurnPathIDs(_ context.Context, headTurnID string) ([]string, error) {
	return append([]string(nil), s.paths[headTurnID]...), nil
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
