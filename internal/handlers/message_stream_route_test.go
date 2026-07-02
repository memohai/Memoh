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

func TestLiveMessageVisibilityAllowsDescendantsOfSelectedHead(t *testing.T) {
	t.Parallel()

	visible := map[string]struct{}{
		"turn-a": {},
		"turn-b": {},
	}
	liveHeads := map[string]struct{}{"turn-b": {}}
	graph := messagepkg.SessionTurnGraph{
		DefaultHeadTurnID: "turn-e",
		HeadTurnIDs:       []string{"turn-c", "turn-e"},
		Nodes: []messagepkg.SessionTurnGraphNode{
			{TurnID: "turn-a"},
			{TurnID: "turn-b", ParentTurnID: "turn-a"},
			{TurnID: "turn-c", ParentTurnID: "turn-a"},
			{TurnID: "turn-d", ParentTurnID: "turn-b"},
			{TurnID: "turn-e", ParentTurnID: "turn-d"},
		},
	}

	if got := addVisibleDescendantPathFromGraph(visible, liveHeads, graph, "turn-d"); got != liveTurnVisible {
		t.Fatal("descendant of selected head was filtered")
	}
	if _, ok := visible["turn-d"]; !ok {
		t.Fatalf("descendant was not added to visible set: %#v", visible)
	}
	if got := addVisibleDescendantPathFromGraph(visible, liveHeads, graph, "turn-e"); got != liveTurnVisible {
		t.Fatal("descendant of previously accepted live head was filtered")
	}
	if _, ok := visible["turn-e"]; !ok {
		t.Fatalf("second descendant was not added to visible set: %#v", visible)
	}
	if got := addVisibleDescendantPathFromGraph(visible, liveHeads, graph, "turn-c"); got != liveTurnHidden {
		t.Fatal("sibling branch was treated as selected-path descendant")
	}
	if got := addVisibleDescendantPathFromGraph(visible, liveHeads, graph, "turn-pending"); got != liveTurnStale {
		t.Fatalf("pending live turn missing from graph = %v, want stale", got)
	}
}

func TestVisibleTurnIDSetForHeadRejectsInvalidRequestedHead(t *testing.T) {
	t.Parallel()

	graph := messagepkg.SessionTurnGraph{
		DefaultHeadTurnID: "turn-b",
		HeadTurnIDs:       []string{"turn-b"},
		Nodes: []messagepkg.SessionTurnGraphNode{
			{TurnID: "turn-a"},
			{TurnID: "turn-b", ParentTurnID: "turn-a"},
		},
	}
	visible := visibleTurnIDSetForHead(graph, "missing")
	if len(visible) == 0 {
		t.Fatalf("invalid requested head must not fall through to legacy all-visible mode")
	}
	if _, ok := visible["turn-a"]; ok {
		t.Fatalf("visible included parent from default head after invalid request: %#v", visible)
	}
	if _, ok := visible["turn-b"]; ok {
		t.Fatalf("visible included default head after invalid request: %#v", visible)
	}
	if _, ok := visible["missing"]; ok {
		t.Fatalf("visible included invalid requested head: %#v", visible)
	}
}

func TestVisibleTurnIDSetForHeadRejectsNonLeafRequestedHead(t *testing.T) {
	t.Parallel()

	graph := messagepkg.SessionTurnGraph{
		DefaultHeadTurnID: "turn-c",
		HeadTurnIDs:       []string{"turn-c"},
		Nodes: []messagepkg.SessionTurnGraphNode{
			{TurnID: "turn-a"},
			{TurnID: "turn-b", ParentTurnID: "turn-a"},
			{TurnID: "turn-c", ParentTurnID: "turn-b"},
		},
	}
	if got := resolvedHeadTurnIDForGraph(graph, "turn-b"); got != "" {
		t.Fatalf("resolved stale requested head = %q, want empty", got)
	}
	visible := visibleTurnIDSetForHead(graph, "turn-b")
	if len(visible) == 0 {
		t.Fatalf("stale requested head must not fall through to legacy all-visible mode")
	}
	for _, turnID := range []string{"turn-a", "turn-b", "turn-c"} {
		if _, ok := visible[turnID]; ok {
			t.Fatalf("visible included %s after stale requested head: %#v", turnID, visible)
		}
	}
}

func TestVisibleTurnIDSetForHeadUsesDefaultWhenUnspecified(t *testing.T) {
	t.Parallel()

	graph := messagepkg.SessionTurnGraph{
		DefaultHeadTurnID: "turn-b",
		HeadTurnIDs:       []string{"turn-b"},
		Nodes: []messagepkg.SessionTurnGraphNode{
			{TurnID: "turn-a"},
			{TurnID: "turn-b", ParentTurnID: "turn-a"},
		},
	}
	visible := visibleTurnIDSetForHead(graph, "")
	if _, ok := visible["turn-a"]; !ok {
		t.Fatalf("visible did not include parent turn: %#v", visible)
	}
	if _, ok := visible["turn-b"]; !ok {
		t.Fatalf("visible did not include default head turn: %#v", visible)
	}
}

func TestValidatedSessionTurnGraphRejectsStaleExplicitHead(t *testing.T) {
	t.Parallel()

	h := &MessageHandler{messageService: &graphOnlyMessageService{
		graph: messagepkg.SessionTurnGraph{
			DefaultHeadTurnID: "turn-c",
			HeadTurnIDs:       []string{"turn-c"},
			Nodes: []messagepkg.SessionTurnGraphNode{
				{TurnID: "turn-a"},
				{TurnID: "turn-b", ParentTurnID: "turn-a"},
				{TurnID: "turn-c", ParentTurnID: "turn-b"},
			},
		},
	}}
	_, err := h.validatedSessionTurnGraph(context.Background(), "session-1", "turn-b", false)
	if err == nil {
		t.Fatal("validatedSessionTurnGraph() err = nil, want stale HTTP error")
	}
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusConflict {
		t.Fatalf("HTTPError.Code = %d, want 409", httpErr.Code)
	}
}

func TestValidatedSessionTurnGraphUsesHeadValidator(t *testing.T) {
	t.Parallel()

	svc := &headValidatorMessageService{ok: true}
	h := &MessageHandler{messageService: svc}
	if _, err := h.validatedSessionTurnGraph(context.Background(), "session-1", "turn-b", false); err != nil {
		t.Fatalf("validatedSessionTurnGraph() error = %v", err)
	}
	if svc.validatorCalls != 1 {
		t.Fatalf("validator calls = %d, want 1", svc.validatorCalls)
	}
	if svc.graphCalls != 0 {
		t.Fatalf("graph calls = %d, want 0", svc.graphCalls)
	}
}

func TestValidatedSessionTurnGraphValidatorRejectsStaleHead(t *testing.T) {
	t.Parallel()

	svc := &headValidatorMessageService{ok: false}
	h := &MessageHandler{messageService: svc}
	_, err := h.validatedSessionTurnGraph(context.Background(), "session-1", "turn-b", false)
	if err == nil {
		t.Fatal("validatedSessionTurnGraph() err = nil, want stale HTTP error")
	}
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *echo.HTTPError", err)
	}
	if httpErr.Code != http.StatusConflict {
		t.Fatalf("HTTPError.Code = %d, want 409", httpErr.Code)
	}
	if svc.graphCalls != 0 {
		t.Fatalf("graph calls = %d, want 0", svc.graphCalls)
	}
}

type graphOnlyMessageService struct {
	messagepkg.Service
	graph messagepkg.SessionTurnGraph
}

func (s *graphOnlyMessageService) GetSessionTurnGraph(context.Context, string) (messagepkg.SessionTurnGraph, error) {
	return s.graph, nil
}

type headValidatorMessageService struct {
	messagepkg.Service
	ok             bool
	validatorCalls int
	graphCalls     int
}

func (s *headValidatorMessageService) IsSessionTurnHead(context.Context, string, string) (bool, error) {
	s.validatorCalls++
	return s.ok, nil
}

func (s *headValidatorMessageService) GetSessionTurnGraph(context.Context, string) (messagepkg.SessionTurnGraph, error) {
	s.graphCalls++
	return messagepkg.SessionTurnGraph{}, nil
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
