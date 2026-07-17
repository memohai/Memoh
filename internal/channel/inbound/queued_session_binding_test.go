package inbound

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

const nextDeliverySessionID = "66666666-6666-6666-6666-666666666666"

type queuedSessionBindingWriter struct {
	mu        sync.Mutex
	persisted []messagepkg.PersistInput
}

func (w *queuedSessionBindingWriter) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.persisted = append(w.persisted, input)
	return messagepkg.Message{ID: "77777777-7777-7777-7777-777777777777"}, nil
}

func (*queuedSessionBindingWriter) CompletePendingDelivery(context.Context, string) error {
	return nil
}

func (w *queuedSessionBindingWriter) persistedSessions() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]string, len(w.persisted))
	for i := range w.persisted {
		result[i] = w.persisted[i].SessionID
	}
	return result
}

func TestQueuedReplayDoesNotInjectNewActiveSessionIntoOriginalSession(t *testing.T) {
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	writer := &queuedSessionBindingWriter{}
	started := make(chan conversation.ChatRequest, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	gateway := &fakeChatGateway{onChat: func(req conversation.ChatRequest) {
		if req.SessionID == deliverySessionID {
			started <- req
			<-release
		}
	}}
	dispatcher := NewRouteDispatcher(nil)
	processor := NewChannelInboundProcessor(nil, nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{
		activeSession: SessionResult{ID: nextDeliverySessionID, Type: sessionpkg.TypeChat, Runtime: sessionpkg.RuntimeModel},
		sessions: map[string]SessionResult{
			deliverySessionID:     {ID: deliverySessionID, Type: sessionpkg.TypeChat, Runtime: sessionpkg.RuntimeModel},
			nextDeliverySessionID: {ID: nextDeliverySessionID, Type: sessionpkg.TypeChat, Runtime: sessionpkg.RuntimeModel},
		},
	})
	processor.SetDispatcher(dispatcher)
	t.Cleanup(processor.Close)

	dispatcher.MarkActive("route")
	dispatcher.Enqueue("route", QueuedTask{
		Ctx:                    deliveryContext(),
		Cfg:                    deliveryConfig(),
		Msg:                    queuedDeliveryMessage(),
		Sender:                 &fakeReplySender{},
		PersistedUserMessageID: "55555555-5555-5555-5555-555555555555",
		SessionID:              deliverySessionID,
	})
	drainDone := make(chan struct{})
	go func() {
		processor.drainQueue(deliveryContext(), "route")
		close(drainDone)
	}()

	var original conversation.ChatRequest
	select {
	case original = <-started:
	case <-time.After(time.Second):
		t.Fatal("queued replay did not start in the original session")
	}
	if original.SessionID != deliverySessionID || original.InjectCh == nil {
		t.Fatalf("queued replay session/inject channel = %q/%v, want original session with inject channel", original.SessionID, original.InjectCh)
	}

	next := deliveryChatMessage()
	next.Message.ID = "88888888-8888-8888-8888-888888888888"
	next.Message.Text = "new session message"
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), next, &fakeReplySender{}); err != nil {
		t.Fatalf("new-session HandleInbound() error = %v", err)
	}
	select {
	case injected := <-original.InjectCh:
		t.Fatalf("new-session message was injected into original session: %q", injected.Text)
	default:
	}
	if got := writer.persistedSessions(); len(got) != 1 || got[0] != nextDeliverySessionID {
		t.Fatalf("queued new-session persistence = %v, want [%s]", got, nextDeliverySessionID)
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case <-drainDone:
	case <-time.After(time.Second):
		t.Fatal("queued session chain did not drain")
	}
}
