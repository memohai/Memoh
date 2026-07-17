package inbound

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type transientQueuedGateway struct {
	*fakeChatGateway
	calls atomic.Int32
}

type deletedQueuedSessionEnsurer struct {
	*fakeSessionEnsurer
	deleted bool
}

func (e *deletedQueuedSessionEnsurer) GetSession(ctx context.Context, sessionID string) (SessionResult, error) {
	if e.deleted {
		return SessionResult{}, sessionpkg.ErrNotFound
	}
	return e.fakeSessionEnsurer.GetSession(ctx, sessionID)
}

func (g *transientQueuedGateway) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	if g.calls.Add(1) == 1 {
		chunks := make(chan conversation.StreamChunk)
		errs := make(chan error, 1)
		errs <- errors.New("transient gateway failure")
		close(chunks)
		close(errs)
		return chunks, errs
	}
	return g.fakeChatGateway.StreamChat(ctx, req)
}

func TestQueuedEventReplayUsesPersistedUserMessageWithoutReingestion(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	queuedLeaseValue, claimed := processor.inflightEventDelivery.Load(deliveryEventID)
	if !claimed {
		t.Fatal("queued event delivery claim was released before replay")
	}
	queuedClaim, ok := queuedLeaseValue.(*pipelinepkg.EventDeliveryLease).DeliveryClaim()
	if !ok {
		t.Fatal("queued event delivery claim is unavailable")
	}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); !errors.Is(err, ErrEventDeliveryInFlight) {
		t.Fatalf("waiting provider retry HandleInbound() error = %v, want in-flight", err)
	}
	if gatewayCalls != 0 {
		t.Fatalf("gateway calls before queued replay = %d, want 0", gatewayCalls)
	}
	if writer.calls != 1 {
		t.Fatalf("history writes before drain = %d, want 1", writer.calls)
	}
	assertDeliveryNodeCount(t, pipeline, 1)

	processor.drainQueue(deliveryContext(), "route")

	if gatewayCalls != 1 {
		t.Fatalf("gateway calls after queue drain = %d, want 1", gatewayCalls)
	}
	if !gateway.gotReq.UserMessagePersisted || gateway.gotReq.PersistedUserMessageID != "44444444-4444-4444-4444-444444444444" {
		t.Fatalf("queued ChatRequest persistence = %t/%q, want true/persisted message id",
			gateway.gotReq.UserMessagePersisted,
			gateway.gotReq.PersistedUserMessageID,
		)
	}
	if gateway.gotReq.EventID != deliveryEventID {
		t.Fatalf("queued ChatRequest event id = %q, want %q", gateway.gotReq.EventID, deliveryEventID)
	}
	if gateway.gotReq.EventDeliveryClaim == nil ||
		gateway.gotReq.EventDeliveryClaim.EventID != queuedClaim.EventID ||
		gateway.gotReq.EventDeliveryClaim.ClaimToken != queuedClaim.ClaimToken {
		t.Fatalf("queued ChatRequest delivery claim = %#v, want %#v", gateway.gotReq.EventDeliveryClaim, queuedClaim)
	}
	if writer.calls != 1 {
		t.Fatalf("history writes after drain = %d, want 1", writer.calls)
	}
	assertDeliveryNodeCount(t, pipeline, 1)
}

func TestQueuedEventReplayUsesSessionCapturedAtEnqueue(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	ensurer := &fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Type:    sessionpkg.TypeChat,
		Runtime: sessionpkg.RuntimeModel,
	}, sessions: map[string]SessionResult{deliverySessionID: {
		ID:      deliverySessionID,
		Type:    sessionpkg.TypeChat,
		Runtime: sessionpkg.RuntimeModel,
	}}}
	processor.SetSessionEnsurer(ensurer)
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	ensurer.activeSession = SessionResult{
		ID:      "66666666-6666-6666-6666-666666666666",
		Type:    sessionpkg.TypeDiscuss,
		Runtime: "acp",
	}

	processor.drainQueue(deliveryContext(), "route")

	if gateway.gotReq.SessionID != deliverySessionID {
		t.Fatalf("queued ChatRequest session = %q, want enqueue session %q", gateway.gotReq.SessionID, deliverySessionID)
	}
}

func TestQueuedEventReplayTerminatesWhenCapturedSessionWasDeleted(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	processor.queueRetryDelay = func(int) time.Duration { return time.Hour }
	ensurer := &deletedQueuedSessionEnsurer{fakeSessionEnsurer: &fakeSessionEnsurer{
		activeSession: SessionResult{
			ID:      deliverySessionID,
			Type:    sessionpkg.TypeChat,
			Runtime: sessionpkg.RuntimeModel,
		},
	}}
	processor.SetSessionEnsurer(ensurer)
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	ensurer.deleted = true
	processor.drainQueue(deliveryContext(), "route")

	if writer.completionCalls != 1 || !queries.deliveryIsCompleted() {
		t.Fatalf("deleted-session completion calls/completed = %d/%t, want 1/true", writer.completionCalls, queries.deliveryIsCompleted())
	}
	if dispatcher.IsActive("route") {
		t.Fatal("deleted queued session kept the route active")
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("deleted queued session reached gateway with query %q", gateway.gotReq.Query)
	}
}

func TestQueuedEventHistoryFailureDoesNotEnqueue(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline, failures: 1}
	processor, dispatcher, _ := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err == nil {
		t.Fatal("HandleInbound() error = nil, want queued history failure")
	}
	result := dispatcher.MarkDone("route")
	if len(result.QueuedTasks) != 0 {
		t.Fatalf("queued tasks after history failure = %d, want 0", len(result.QueuedTasks))
	}
}

func TestQueuedEventProviderRetryRecoversLostInMemoryQueueAfterLeaseExpiry(t *testing.T) {
	queries := &durableEventQueries{}
	firstPipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	firstWriter := &durableHistoryWriter{queries: queries, pipeline: firstPipeline}
	firstProcessor, firstDispatcher, _ := newQueuedEventDeliveryProcessor(t, queries, firstWriter, firstPipeline)
	firstDispatcher.MarkActive("route")

	if err := firstProcessor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("initial enqueue HandleInbound() error = %v", err)
	}
	pending, persistedID := queries.pendingHistoryState()
	if !pending || persistedID != "44444444-4444-4444-4444-444444444444" {
		t.Fatalf("initial queued history = pending:%t id:%q", pending, persistedID)
	}
	firstClaim, loaded := firstProcessor.inflightEventDelivery.Load(deliveryEventID)
	if !loaded {
		t.Fatal("initial queued delivery lease is missing")
	}
	firstLease, ok := firstClaim.(*pipelinepkg.EventDeliveryLease)
	if !ok {
		t.Fatalf("initial queued delivery lease type = %T", firstClaim)
	}
	defer func() { _ = firstLease.Release(deliveryContext()) }()
	firstDeliveryClaim, ok := firstLease.DeliveryClaim()
	if !ok {
		t.Fatal("initial queued delivery claim is unavailable")
	}
	queries.expireDeliveryClaim()

	retryPipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	retryWriter := &durableHistoryWriter{queries: queries, pipeline: retryPipeline}
	retryProcessor, retryDispatcher, retryGateway := newQueuedEventDeliveryProcessor(t, queries, retryWriter, retryPipeline)
	retryDispatcher.MarkActive("route")
	if err := retryProcessor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("provider retry HandleInbound() error = %v", err)
	}
	if retryWriter.calls != 0 {
		t.Fatalf("provider retry history writes = %d, want 0", retryWriter.calls)
	}
	retryClaimValue, loaded := retryProcessor.inflightEventDelivery.Load(deliveryEventID)
	if !loaded {
		t.Fatal("provider retry delivery lease is missing")
	}
	retryDeliveryClaim, ok := retryClaimValue.(*pipelinepkg.EventDeliveryLease).DeliveryClaim()
	if !ok {
		t.Fatal("provider retry delivery claim is unavailable")
	}
	if retryDeliveryClaim.ClaimToken == firstDeliveryClaim.ClaimToken {
		t.Fatal("provider retry reused the expired delivery claim token")
	}

	retryGatewayCalls := 0
	retryGateway.onChat = func(conversation.ChatRequest) { retryGatewayCalls++ }
	retryProcessor.drainQueue(deliveryContext(), "route")
	if retryGatewayCalls != 1 || retryGateway.gotReq.PersistedUserMessageID != persistedID {
		t.Fatalf("recovered queue gateway calls/id = %d/%q, want 1/%q", retryGatewayCalls, retryGateway.gotReq.PersistedUserMessageID, persistedID)
	}
	if retryGateway.gotReq.EventDeliveryClaim == nil ||
		retryGateway.gotReq.EventDeliveryClaim.EventID != retryDeliveryClaim.EventID ||
		retryGateway.gotReq.EventDeliveryClaim.ClaimToken != retryDeliveryClaim.ClaimToken {
		t.Fatalf("recovered queue delivery claim = %#v, want %#v", retryGateway.gotReq.EventDeliveryClaim, retryDeliveryClaim)
	}
	if retryWriter.completionCalls != 1 {
		t.Fatalf("recovered queue completion writes = %d, want 1", retryWriter.completionCalls)
	}

	completedPipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	completedWriter := &durableHistoryWriter{queries: queries, pipeline: completedPipeline}
	completedProcessor, _, completedGateway := newQueuedEventDeliveryProcessor(t, queries, completedWriter, completedPipeline)
	completedCalls := 0
	completedGateway.onChat = func(conversation.ChatRequest) { completedCalls++ }
	if err := completedProcessor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("completed provider retry HandleInbound() error = %v", err)
	}
	if completedCalls != 0 || completedWriter.calls != 0 {
		t.Fatalf("completed provider retry calls = gateway:%d history:%d, want 0/0", completedCalls, completedWriter.calls)
	}
}

func TestQueuedEventTransientFailureRetriesWithoutProviderRedelivery(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, baseGateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	gateway := &transientQueuedGateway{fakeChatGateway: baseGateway}
	processor.runner = gateway
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	processor.drainQueue(deliveryContext(), "route")

	deadline := time.Now().Add(2 * time.Second)
	for gateway.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := gateway.calls.Load(); got != 2 {
		t.Fatalf("gateway calls = %d, want one failed attempt and one self-driven retry", got)
	}
	for !queries.deliveryIsCompleted() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("queued delivery did not complete after transient retry")
	}
	if dispatcher.IsActive("route") {
		t.Fatal("route remained active after retried queue drained")
	}
}

func TestQueuedReplayTreatsExternallyCompletedDeliveryAsSuccess(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	dispatcher.MarkActive("route")
	task := QueuedTask{
		Ctx:                    deliveryContext(),
		Cfg:                    deliveryConfig(),
		Msg:                    queuedDeliveryMessage(),
		Sender:                 &fakeReplySender{},
		PersistedUserMessageID: queries.historyID.String(),
		EventID:                deliveryEventID,
		SessionID:              deliverySessionID,
	}

	if err := processor.HandleInbound(withQueuedReplayState(deliveryContext(), task), task.Cfg, task.Msg, task.Sender); err != nil {
		t.Fatalf("completed queued replay error = %v", err)
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("gateway request = %#v, want no replay for completed delivery", gateway.gotReq)
	}
}

func TestQueuedReplayRetainsPoisonTaskAtHeadOfLine(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
	processor.queueRetryDelay = func(int) time.Duration { return time.Millisecond }
	gateway.err = errors.New("permanent gateway failure")
	var calls atomic.Int32
	gateway.onChat = func(conversation.ChatRequest) { calls.Add(1) }
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	processor.drainQueue(deliveryContext(), "route")
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 10 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := calls.Load(); got < 10 {
		t.Fatalf("gateway calls = %d, want continued retries", got)
	}
	if !dispatcher.IsActive("route") {
		t.Fatal("poison task released route ownership while still incomplete")
	}
	if _, claimed := processor.inflightEventDelivery.Load(deliveryEventID); !claimed {
		t.Fatal("poison task released its event delivery claim while still incomplete")
	}
}

func TestQueuedRetryShutdownReleasesRouteAndDeliveryClaims(t *testing.T) {
	const nextEventID = "66666666-6666-6666-6666-666666666666"
	currentQueries := &durableEventQueries{}
	nextQueries := &durableEventQueries{}
	currentLease, err := pipelinepkg.NewEventStore(nil, currentQueries).ClaimEventDelivery(
		context.Background(),
		deliveryEventID,
	)
	if err != nil || currentLease == nil {
		t.Fatalf("claim current delivery = %#v, %v", currentLease, err)
	}
	nextLease, err := pipelinepkg.NewEventStore(nil, nextQueries).ClaimEventDelivery(
		context.Background(),
		nextEventID,
	)
	if err != nil || nextLease == nil {
		t.Fatalf("claim next delivery = %#v, %v", nextLease, err)
	}

	dispatcher := NewRouteDispatcher(slog.Default())
	processor := NewChannelInboundProcessor(slog.Default(), nil, nil, nil, nil, nil, nil, "", 0)
	processor.SetDispatcher(dispatcher)
	processor.queueRetryDelay = func(int) time.Duration { return time.Hour }
	processor.inflightEventDelivery.Store(deliveryEventID, currentLease)
	processor.inflightEventDelivery.Store(nextEventID, nextLease)
	dispatcher.MarkActive("route")
	dispatcher.Enqueue("route", QueuedTask{Ctx: context.Background(), EventID: nextEventID})
	processor.startQueuedRetry(context.Background(), "route", QueuedTask{
		Ctx:     context.Background(),
		EventID: deliveryEventID,
	})

	closed := make(chan struct{})
	go func() {
		processor.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("processor Close did not wait for queued retry shutdown")
	}
	if dispatcher.IsActive("route") {
		t.Fatal("queued route remained active after processor shutdown")
	}
	for name, queries := range map[string]*durableEventQueries{
		"current": currentQueries,
		"next":    nextQueries,
	} {
		queries.mu.Lock()
		claimed := queries.claimToken.Valid
		queries.mu.Unlock()
		if claimed {
			t.Fatalf("%s delivery claim remained held after processor shutdown", name)
		}
	}
}

func TestRouteDispatcherDeduplicatesQueuedEventID(t *testing.T) {
	dispatcher := NewRouteDispatcher(slog.Default())
	dispatcher.Enqueue("route", QueuedTask{EventID: deliveryEventID, Text: "first"})
	dispatcher.Enqueue("route", QueuedTask{EventID: deliveryEventID, Text: "duplicate"})

	result := dispatcher.MarkDone("route")
	if len(result.QueuedTasks) != 1 || result.QueuedTasks[0].Text != "first" {
		t.Fatalf("deduplicated queued tasks = %#v, want first task only", result.QueuedTasks)
	}
}

func TestQueuedReplaySuppressesProviderRetryWhileGatewayInFlight(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	dispatcher := NewRouteDispatcher(slog.Default())
	gateway := &blockingDeliveryGateway{started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(gateway.release) })
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Type:    sessionpkg.TypeChat,
		Runtime: sessionpkg.RuntimeModel,
	}})
	processor.SetPipeline(pipeline, pipelinepkg.NewEventStore(nil, queries), nil)
	processor.SetDispatcher(dispatcher)
	t.Cleanup(processor.Close)
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}

	drainDone := make(chan struct{})
	go func() {
		processor.drainQueue(deliveryContext(), "route")
		close(drainDone)
	}()
	<-gateway.started

	retryDone := make(chan error, 1)
	go func() {
		retryDone <- processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedDeliveryMessage(), &fakeReplySender{})
	}()
	select {
	case err := <-retryDone:
		if !errors.Is(err, ErrEventDeliveryInFlight) {
			t.Fatalf("provider retry HandleInbound() error = %v, want in-flight", err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider retry did not return while queued replay was in flight")
	}
	if got := gateway.calls.Load(); got != 1 {
		t.Fatalf("gateway calls while queued replay is in flight = %d, want 1", got)
	}

	releaseOnce.Do(func() { close(gateway.release) })
	select {
	case <-drainDone:
	case <-time.After(time.Second):
		t.Fatal("queued replay did not return after provider release")
	}
	deadline := time.Now().Add(time.Second)
	for gateway.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := gateway.calls.Load(); got != 2 {
		t.Fatalf("gateway calls after transient failure = %d, want self-driven retry", got)
	}
	for time.Now().Before(deadline) {
		if _, claimed := processor.inflightEventDelivery.Load(deliveryEventID); !claimed {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("queued event delivery claim was retained after successful retry")
}

func newQueuedEventDeliveryProcessor(
	t *testing.T,
	queries *durableEventQueries,
	writer messagepkg.Writer,
	pipeline *pipelinepkg.Pipeline,
) (*ChannelInboundProcessor, *RouteDispatcher, *fakeChatGateway) {
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	gateway := &fakeChatGateway{}
	dispatcher := NewRouteDispatcher(slog.Default())
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Type:    sessionpkg.TypeChat,
		Runtime: sessionpkg.RuntimeModel,
	}})
	processor.SetPipeline(pipeline, pipelinepkg.NewEventStore(nil, queries), nil)
	processor.SetDispatcher(dispatcher)
	t.Cleanup(processor.Close)
	return processor, dispatcher, gateway
}

func queuedDeliveryMessage() channel.InboundMessage {
	msg := deliveryChatMessage()
	msg.Message.Text = "/next queued"
	return msg
}
