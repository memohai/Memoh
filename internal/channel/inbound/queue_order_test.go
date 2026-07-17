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
)

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

func TestQueuedPumpKeepsFIFOWhenMessageArrivesDuringFirstReplay(t *testing.T) {
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	writer := &queueOrderWriter{fakeChatService: routes}
	baseGateway := &fakeChatGateway{}
	gateway := &queueOrderGateway{
		fakeChatGateway: baseGateway,
		b1Started:       make(chan struct{}),
		releaseB1:       make(chan struct{}),
	}
	dispatcher := NewRouteDispatcher(slog.Default())
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Runtime: "model",
	}})
	processor.SetDispatcher(dispatcher)
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
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	writer := &queueOrderWriter{fakeChatService: routes}
	gateway := &shutdownBlockingQueueGateway{
		fakeChatGateway: &fakeChatGateway{},
		started:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	dispatcher := NewRouteDispatcher(slog.Default())
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Runtime: "model",
	}})
	processor.SetDispatcher(dispatcher)
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
