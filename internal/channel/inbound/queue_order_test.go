package inbound

import (
	"context"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	messagepkg "github.com/memohai/memoh/internal/message"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

const nextDeliverySessionID = "66666666-6666-6666-6666-666666666666"

type queueOrderWriter struct {
	*fakeChatService
}

func (*queueOrderWriter) CompletePendingDelivery(context.Context, string) error {
	return nil
}

type queueOrderGateway struct {
	*fakeChatGateway
	mu        sync.Mutex
	order     []string
	b1Started chan struct{}
	releaseB1 chan struct{}
}

type shutdownBlockingQueueGateway struct {
	*fakeChatGateway
	started chan struct{}
	release chan struct{}
}

func (g *shutdownBlockingQueueGateway) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	close(g.started)
	<-g.release
	return g.fakeChatGateway.StreamChat(ctx, req)
}

func (g *queueOrderGateway) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	g.mu.Lock()
	g.order = append(g.order, req.ExternalMessageID)
	g.mu.Unlock()
	if req.ExternalMessageID == "B1" {
		close(g.b1Started)
		select {
		case <-g.releaseB1:
		case <-ctx.Done():
		}
	}
	return g.fakeChatGateway.StreamChat(ctx, req)
}

func (g *queueOrderGateway) processedOrder() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.order...)
}

func newQueueOrderProcessor(gateway flow.Runner) (*ChannelInboundProcessor, *RouteDispatcher) {
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	writer := &queueOrderWriter{fakeChatService: routes}
	dispatcher := NewRouteDispatcher(slog.Default())
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Runtime: "model",
	}})
	processor.SetDispatcher(dispatcher)
	return processor, dispatcher
}

func TestQueuedPumpKeepsFIFOWhenMessageArrivesDuringFirstReplay(t *testing.T) {
	gateway := &queueOrderGateway{
		fakeChatGateway: &fakeChatGateway{},
		b1Started:       make(chan struct{}),
		releaseB1:       make(chan struct{}),
	}
	processor, dispatcher := newQueueOrderProcessor(gateway)
	t.Cleanup(processor.Close)
	dispatcher.MarkActive("route")

	enqueue := func(id string) {
		msg := queuedDeliveryMessage()
		msg.Message.ID = id
		if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), msg, &fakeReplySender{}); err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
	}
	enqueue("B1")
	enqueue("B2")

	drained := make(chan struct{})
	go func() {
		processor.drainQueue(deliveryContext(), "route")
		close(drained)
	}()
	select {
	case <-gateway.b1Started:
	case <-time.After(time.Second):
		t.Fatal("B1 did not start")
	}
	enqueue("B3")
	close(gateway.releaseB1)
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("queue did not drain")
	}

	if got, want := gateway.processedOrder(), []string{"B1", "B2", "B3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("processed order = %#v, want %#v", got, want)
	}
}

func TestProcessorCloseWaitsForSynchronousQueuedPump(t *testing.T) {
	gateway := &shutdownBlockingQueueGateway{
		fakeChatGateway: &fakeChatGateway{},
		started:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	processor, dispatcher := newQueueOrderProcessor(gateway)
	dispatcher.MarkActive("route")
	msg := queuedDeliveryMessage()
	msg.Message.ID = "B1"
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), msg, &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue B1: %v", err)
	}

	drained := make(chan struct{})
	go func() {
		processor.drainQueue(deliveryContext(), "route")
		close(drained)
	}()
	select {
	case <-gateway.started:
	case <-time.After(time.Second):
		t.Fatal("synchronous queued pump did not start")
	}
	closed := make(chan struct{})
	go func() {
		processor.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("processor Close returned while synchronous queued pump was still running")
	case <-time.After(25 * time.Millisecond):
	}
	close(gateway.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("processor Close did not return after synchronous queued pump stopped")
	}
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("synchronous queued pump did not return")
	}
	if dispatcher.IsActive("route") {
		t.Fatal("route remained active after synchronous queued pump shutdown")
	}
}

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
