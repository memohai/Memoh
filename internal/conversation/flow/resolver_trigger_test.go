package flow

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/session"
)

type fakeBackgroundSessionService struct {
	getFn func(ctx context.Context, sessionID string) (session.Session, error)
}

func (f *fakeBackgroundSessionService) Get(ctx context.Context, sessionID string) (session.Session, error) {
	if f == nil || f.getFn == nil {
		return session.Session{}, errors.New("unexpected Get call")
	}
	return f.getFn(ctx, sessionID)
}

func (*fakeBackgroundSessionService) UpdateTitle(context.Context, string, string) (session.Session, error) {
	return session.Session{}, errors.New("unexpected UpdateTitle call")
}

type fakeBackgroundRouteService struct {
	getByIDFn func(ctx context.Context, routeID string) (route.Route, error)
}

func (f *fakeBackgroundRouteService) GetByID(ctx context.Context, routeID string) (route.Route, error) {
	if f == nil || f.getByIDFn == nil {
		return route.Route{}, errors.New("unexpected GetByID call")
	}
	return f.getByIDFn(ctx, routeID)
}

func TestResolveBackgroundDeliveryContext_RouteBackedSession(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		logger: slog.Default(),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				if sessionID != "session-1" {
					t.Fatalf("unexpected session id: %s", sessionID)
				}
				return session.Session{
					ID:          sessionID,
					BotID:       "bot-1",
					RouteID:     "route-1",
					ChannelType: "telegram",
				}, nil
			},
		},
		routeService: &fakeBackgroundRouteService{
			getByIDFn: func(_ context.Context, routeID string) (route.Route, error) {
				if routeID != "route-1" {
					t.Fatalf("unexpected route id: %s", routeID)
				}
				return route.Route{
					ID:          routeID,
					Platform:    "telegram",
					ReplyTarget: "chat-42",
				}, nil
			},
		},
	}

	delivery, err := resolver.resolveBackgroundDeliveryContext(context.Background(), "bot-1", "session-1")
	if err != nil {
		t.Fatalf("resolveBackgroundDeliveryContext returned error: %v", err)
	}
	if delivery.routeID != "route-1" {
		t.Fatalf("unexpected route id: %q", delivery.routeID)
	}
	if delivery.channelType != "telegram" {
		t.Fatalf("unexpected channel type: %q", delivery.channelType)
	}
	if delivery.replyTarget != "chat-42" {
		t.Fatalf("unexpected reply target: %q", delivery.replyTarget)
	}
}

func TestResolveBackgroundDeliveryContext_LocalSessionFallback(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		logger: slog.Default(),
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, sessionID string) (session.Session, error) {
				return session.Session{
					ID:          sessionID,
					BotID:       "bot-1",
					ChannelType: "local",
				}, nil
			},
		},
	}

	delivery, err := resolver.resolveBackgroundDeliveryContext(context.Background(), "bot-1", "session-1")
	if err != nil {
		t.Fatalf("resolveBackgroundDeliveryContext returned error: %v", err)
	}
	if delivery.routeID != "" {
		t.Fatalf("expected empty route id, got %q", delivery.routeID)
	}
	if delivery.channelType != "local" {
		t.Fatalf("unexpected channel type: %q", delivery.channelType)
	}
	if delivery.replyTarget != "bot-1" {
		t.Fatalf("unexpected reply target: %q", delivery.replyTarget)
	}
}

func TestTriggerBackgroundNotification_RequeuesWholeBatchOnDeliveryContextFailure(t *testing.T) {
	t.Parallel()

	bgMgr := background.New(nil)
	batch := []background.Notification{
		{TaskID: "task-1", BotID: "bot-1", SessionID: "session-1", Status: background.TaskCompleted, Command: "cmd-1"},
		{TaskID: "task-2", BotID: "bot-1", SessionID: "session-1", Status: background.TaskFailed, Command: "cmd-2"},
	}
	bgMgr.RequeueNotifications(batch)

	resolver := &Resolver{
		logger:    slog.Default(),
		bgManager: bgMgr,
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, _ string) (session.Session, error) {
				return session.Session{}, errors.New("session lookup failed")
			},
		},
	}

	resolver.TriggerBackgroundNotification(context.Background(), "bot-1", "session-1")

	remaining := bgMgr.DrainNotifications("bot-1", "session-1")
	if len(remaining) != len(batch) {
		t.Fatalf("expected %d notifications to be requeued, got %d", len(batch), len(remaining))
	}
	for i, n := range remaining {
		if n.TaskID != batch[i].TaskID {
			t.Fatalf("unexpected task order after requeue at %d: got %q want %q", i, n.TaskID, batch[i].TaskID)
		}
	}
}

func TestTriggerBackgroundNotification_DefersWhileSessionTurnActive(t *testing.T) {
	t.Parallel()

	bgMgr := background.New(nil)
	bgMgr.RequeueNotifications([]background.Notification{{
		TaskID:    "task-1",
		BotID:     "bot-1",
		SessionID: "session-1",
		Status:    background.TaskCompleted,
		Command:   "cmd-1",
	}})

	lookups := make(chan struct{}, 1)
	resolver := &Resolver{
		logger:    slog.Default(),
		bgManager: bgMgr,
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, _ string) (session.Session, error) {
				lookups <- struct{}{}
				return session.Session{}, errors.New("unexpected session lookup")
			},
		},
	}

	doneTurn := resolver.enterSessionTurn(context.Background(), "bot-1", "session-1")
	resolver.TriggerBackgroundNotification(context.Background(), "bot-1", "session-1")

	select {
	case <-lookups:
		t.Fatal("expected trigger to defer while session turn is active")
	case <-time.After(50 * time.Millisecond):
	}

	if !bgMgr.HasNotifications("bot-1", "session-1") {
		t.Fatal("expected notifications to remain queued while session turn is active")
	}

	doneTurn()
}

func TestSessionTurnExit_TriggersPendingBackgroundNotifications(t *testing.T) {
	t.Parallel()

	bgMgr := background.New(nil)
	bgMgr.RequeueNotifications([]background.Notification{{
		TaskID:    "task-1",
		BotID:     "bot-1",
		SessionID: "session-1",
		Status:    background.TaskCompleted,
		Command:   "cmd-1",
	}})

	lookups := make(chan struct{}, 1)
	resolver := &Resolver{
		logger:    slog.Default(),
		bgManager: bgMgr,
		sessionService: &fakeBackgroundSessionService{
			getFn: func(_ context.Context, _ string) (session.Session, error) {
				lookups <- struct{}{}
				return session.Session{}, errors.New("session lookup failed")
			},
		},
	}

	doneTurn := resolver.enterSessionTurn(context.Background(), "bot-1", "session-1")
	resolver.TriggerBackgroundNotification(context.Background(), "bot-1", "session-1")
	doneTurn()

	select {
	case <-lookups:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected idle transition to trigger pending background notifications")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for !bgMgr.HasNotifications("bot-1", "session-1") {
		if time.Now().After(deadline) {
			t.Fatal("expected failed delivery attempt to requeue notifications")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
