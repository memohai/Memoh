package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func newInjectionRedeliveryFixture(t *testing.T) (*ChannelInboundProcessor, *durableEventQueries, *durableHistoryWriter, *fakeChatGateway, *RouteDispatcher) {
	t.Helper()
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	dispatcher := NewRouteDispatcher(nil)
	processor.SetDispatcher(dispatcher)
	return processor, queries, writer, gateway, dispatcher
}

func TestAcceptedButUnconsumedInjectionReleasesDeliveryForRetry(t *testing.T) {
	t.Parallel()

	processor, queries, _, _, dispatcher := newInjectionRedeliveryFixture(t)
	dispatcher.MarkActive("route")
	sender := &fakeReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("first HandleInbound() error = %v", err)
	}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); !errors.Is(err, ErrEventDeliveryInFlight) {
		t.Fatalf("redelivery HandleInbound() error = %v, want in-flight", err)
	}

	processor.drainQueue(deliveryContext(), "route")
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
	waitForDurableEventDeliveryRelease(t, queries)
	if queries.deliveryIsCompleted() {
		t.Fatal("unconsumed injection marked delivery complete")
	}
	injectCh := dispatcher.MarkActive("route")
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("recovery HandleInbound() error = %v", err)
	}
	if got := len(injectCh); got != 1 {
		t.Fatalf("recovery injected messages = %d, want 1", got)
	}
	processor.drainQueue(deliveryContext(), "route")
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
}

func TestPersistedInjectionCompletesOnlyAfterSuccessfulStream(t *testing.T) {
	t.Parallel()

	processor, queries, _, _, dispatcher := newInjectionRedeliveryFixture(t)
	injectCh := dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	injected := <-injectCh
	value, loaded := processor.inflightEventDelivery.Load(deliveryEventID)
	if !loaded {
		t.Fatal("injected delivery lease missing")
	}
	wantClaim, ok := value.(*pipelinepkg.EventDeliveryLease).DeliveryClaim()
	if !ok || injected.Source.DeliveryClaim == nil ||
		injected.Source.EventID != wantClaim.EventID ||
		injected.Source.DeliveryClaim.EventID != wantClaim.EventID ||
		injected.Source.DeliveryClaim.ClaimToken != wantClaim.ClaimToken {
		t.Fatalf("injected delivery claim = event:%q claim:%#v, want %#v", injected.Source.EventID, injected.Source.DeliveryClaim, wantClaim)
	}
	queries.markDeliveryHandled()
	injected.OnPersisted(nil)
	if queries.deliveryIsCompleted() {
		t.Fatal("persisted injection completed before stream finalization")
	}
	processor.drainQueue(deliveryContext(), "route")
	waitForDeliveryCompletion(t, processor, queries, deliveryEventID)
}

func TestLeaseLostPersistedInjectionRedeliveryDoesNotCompleteDuplicate(t *testing.T) {
	t.Parallel()

	processor, queries, _, _, dispatcher := newInjectionRedeliveryFixture(t)
	injectCh := dispatcher.MarkActive("route")
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	streamKey := deliveryBotID + ":route"
	processor.activeStreams.Store(streamKey, cancelStream)
	defer processor.activeStreams.Delete(streamKey)

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	injected := <-injectCh
	queries.markDeliveryHandled()
	injected.OnPersisted(nil)
	value, loaded := processor.inflightEventDelivery.Load(deliveryEventID)
	if !loaded {
		t.Fatal("injected delivery lease missing")
	}
	lease := value.(*pipelinepkg.EventDeliveryLease)
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("release injected lease: %v", err)
	}
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
	select {
	case <-streamCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("owning stream was not canceled after injected delivery lease loss")
	}

	err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{})
	if !errors.Is(err, ErrEventDeliveryInFlight) {
		t.Fatalf("immediate redelivery error = %v, want in-flight", err)
	}
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
	if queries.deliveryIsCompleted() {
		t.Fatal("duplicate injection redelivery marked unfinished response complete")
	}

	processor.drainQueueAfterStream(deliveryContext(), "route", errors.New("stream failed"))
	recoveryInjectCh := dispatcher.MarkActive("route")
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("recovery HandleInbound() error = %v", err)
	}
	if got := len(recoveryInjectCh); got != 1 {
		t.Fatalf("recovery injected messages = %d, want 1", got)
	}
	processor.drainQueue(deliveryContext(), "route")
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
}

func TestPersistedInjectionReplaysAfterStreamFinalizationFails(t *testing.T) {
	t.Parallel()

	processor, queries, writer, gateway, dispatcher := newInjectionRedeliveryFixture(t)
	injectCh := dispatcher.MarkActive("route")
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	sender := &fakeReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	injected := <-injectCh
	queries.markDeliveryHandled()
	injected.OnPersisted(nil)
	processor.drainQueueAfterStream(deliveryContext(), "route", errors.New("close stream failed"))
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
	waitForDurableEventDeliveryRelease(t, queries)
	if queries.deliveryIsCompleted() {
		t.Fatal("failed stream finalization marked injection complete")
	}

	replayContent, err := json.Marshal(conversation.ModelMessage{
		Role:    "assistant",
		Content: conversation.NewTextContent("injected reply"),
	})
	if err != nil {
		t.Fatalf("marshal injected replay: %v", err)
	}
	queries.mu.Lock()
	queries.response = true
	queries.mu.Unlock()
	writer.replayMessages = []messagepkg.Message{{
		ID:      "55555555-5555-5555-5555-555555555555",
		Role:    "assistant",
		Content: replayContent,
	}}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("redelivery HandleInbound() error = %v", err)
	}
	if gatewayCalls != 0 {
		t.Fatalf("gateway calls = %d, want durable injected response replay", gatewayCalls)
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "injected reply" {
		t.Fatalf("replayed replies = %#v, want injected reply", sender.sent)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("redelivered injection was not completed after durable replay")
	}
}

func TestPersistedInjectionReplaysWhileRouteIsActive(t *testing.T) {
	t.Parallel()

	processor, queries, writer, gateway, dispatcher := newInjectionRedeliveryFixture(t)
	injectCh := dispatcher.MarkActive("route")
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	sender := &fakeReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	injected := <-injectCh
	queries.markDeliveryHandled()
	injected.OnPersisted(nil)
	processor.drainQueueAfterStream(deliveryContext(), "route", errors.New("close stream failed"))
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
	waitForDurableEventDeliveryRelease(t, queries)

	replayContent, err := json.Marshal(conversation.ModelMessage{
		Role:    "assistant",
		Content: conversation.NewTextContent("injected reply"),
	})
	if err != nil {
		t.Fatalf("marshal injected replay: %v", err)
	}
	queries.mu.Lock()
	queries.response = true
	queries.mu.Unlock()
	writer.replayMessages = []messagepkg.Message{{
		ID:      "55555555-5555-5555-5555-555555555555",
		Role:    "assistant",
		Content: replayContent,
	}}

	activeInjectCh := dispatcher.MarkActive("route")
	activeCtx, cancelActive := context.WithCancel(context.Background())
	defer cancelActive()
	streamKey := deliveryBotID + ":route"
	processor.activeStreams.Store(streamKey, cancelActive)
	defer processor.activeStreams.Delete(streamKey)
	defer processor.drainQueue(deliveryContext(), "route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("redelivery HandleInbound() error = %v", err)
	}
	if gatewayCalls != 0 {
		t.Fatalf("gateway calls = %d, want durable injected response replay", gatewayCalls)
	}
	if len(activeInjectCh) != 0 {
		t.Fatalf("active route received %d duplicate injections", len(activeInjectCh))
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "injected reply" {
		t.Fatalf("replayed replies = %#v, want injected reply", sender.sent)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("redelivered injection was not completed after durable replay")
	}
	if !dispatcher.IsActive("route") {
		t.Fatal("durable replay released the unrelated active route")
	}
	if _, loaded := processor.activeStreams.Load(streamKey); !loaded {
		t.Fatal("durable replay replaced the unrelated active stream registration")
	}
	select {
	case <-activeCtx.Done():
		t.Fatal("durable replay canceled the unrelated active stream")
	default:
	}
}

func TestPersistedInjectionResponseCommittedBetweenStateReadAndClaimReplays(t *testing.T) {
	t.Parallel()

	processor, queries, writer, gateway, dispatcher := newInjectionRedeliveryFixture(t)
	injectCh := dispatcher.MarkActive("route")
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	sender := &fakeReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	injected := <-injectCh
	queries.markDeliveryHandled()
	injected.OnPersisted(nil)
	processor.drainQueueAfterStream(deliveryContext(), "route", errors.New("close stream failed"))
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
	waitForDurableEventDeliveryRelease(t, queries)

	replayContent, err := json.Marshal(conversation.ModelMessage{
		Role:    "assistant",
		Content: conversation.NewTextContent("injected reply"),
	})
	if err != nil {
		t.Fatalf("marshal injected replay: %v", err)
	}
	queries.mu.Lock()
	queries.responseOnClaim = true
	queries.mu.Unlock()
	writer.replayMessages = []messagepkg.Message{{
		ID:      "55555555-5555-5555-5555-555555555555",
		Role:    "assistant",
		Content: replayContent,
	}}

	activeInjectCh := dispatcher.MarkActive("route")
	defer processor.drainQueue(deliveryContext(), "route")
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("redelivery HandleInbound() error = %v", err)
	}
	if gatewayCalls != 0 {
		t.Fatalf("gateway calls = %d, want durable injected response replay", gatewayCalls)
	}
	if len(activeInjectCh) != 0 {
		t.Fatalf("active route received %d duplicate injections", len(activeInjectCh))
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "injected reply" {
		t.Fatalf("replayed replies = %#v, want injected reply", sender.sent)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("redelivered injection was not completed after durable replay")
	}
}

func TestPersistedInjectionHistoryCommittedWhileClaimWaitsReplays(t *testing.T) {
	t.Parallel()

	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	canonical := pipelinepkg.AdaptInbound(
		pipelineInboundMessage(deliveryConfig(), deliveryChatMessage()),
		deliverySessionID,
		"55555555-5555-5555-5555-555555555555",
		"User",
	)
	seeded, err := pipelinepkg.NewEventStore(nil, queries).PersistEvent(
		deliveryContext(),
		deliveryBotID,
		deliverySessionID,
		canonical,
	)
	if err != nil || !seeded.Inserted {
		t.Fatalf("seed pipeline event = %#v, %v", seeded, err)
	}
	queries.historyResponseOnClaim = true
	replayContent, err := json.Marshal(conversation.ModelMessage{
		Role:    "assistant",
		Content: conversation.NewTextContent("late committed reply"),
	})
	if err != nil {
		t.Fatalf("marshal replay response: %v", err)
	}
	writer := &durableHistoryWriter{
		queries:  queries,
		pipeline: pipeline,
		replayMessages: []messagepkg.Message{{
			ID:      "66666666-6666-6666-6666-666666666666",
			Role:    "assistant",
			Content: replayContent,
		}},
	}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	dispatcher := NewRouteDispatcher(nil)
	activeInjectCh := dispatcher.MarkActive("route")
	processor.SetDispatcher(dispatcher)
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	sender := &fakeReplySender{}
	defer processor.drainQueue(deliveryContext(), "route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if gatewayCalls != 0 || len(activeInjectCh) != 0 || writer.replayCalls != 1 {
		t.Fatalf("late commit calls = gateway:%d inject:%d replay:%d, want 0/0/1", gatewayCalls, len(activeInjectCh), writer.replayCalls)
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "late committed reply" {
		t.Fatalf("replayed replies = %#v", sender.sent)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("late committed response delivery was not completed")
	}
}

func TestCoverageOnlyInjectionCompletesWithoutTouchingActiveRoute(t *testing.T) {
	t.Parallel()

	processor, queries, writer, gateway, dispatcher := newInjectionRedeliveryFixture(t)
	firstInjectCh := dispatcher.MarkActive("route")
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("first HandleInbound() error = %v", err)
	}
	injected := <-firstInjectCh
	queries.markDeliveryHandled()
	injected.OnPersisted(nil)
	processor.drainQueueAfterStream(deliveryContext(), "route", errors.New("stream failed"))
	waitForEventDeliveryRelease(t, processor, deliveryEventID)
	waitForDurableEventDeliveryRelease(t, queries)

	queries.mu.Lock()
	queries.response = true
	queries.responseCoveredOnly = true
	queries.mu.Unlock()
	activeInjectCh := dispatcher.MarkActive("route")
	activeCtx, cancelActive := context.WithCancel(context.Background())
	defer cancelActive()
	streamKey := deliveryBotID + ":route"
	processor.activeStreams.Store(streamKey, cancelActive)
	defer processor.activeStreams.Delete(streamKey)
	defer processor.drainQueue(deliveryContext(), "route")
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	sender := &fakeReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
		t.Fatalf("coverage-only HandleInbound() error = %v", err)
	}
	if gatewayCalls != 0 || len(activeInjectCh) != 0 || len(sender.sent) != 0 || writer.replayCalls != 0 {
		t.Fatalf("coverage-only calls = gateway:%d inject:%d sent:%d replay:%d, want 0/0/0/0",
			gatewayCalls, len(activeInjectCh), len(sender.sent), writer.replayCalls)
	}
	if !queries.deliveryIsCompleted() || !dispatcher.IsActive("route") {
		t.Fatalf("coverage-only completion/route = %t/%t, want true/true", queries.deliveryIsCompleted(), dispatcher.IsActive("route"))
	}
	if _, loaded := processor.activeStreams.Load(streamKey); !loaded {
		t.Fatal("coverage-only completion replaced the unrelated active stream")
	}
	select {
	case <-activeCtx.Done():
		t.Fatal("coverage-only completion canceled the unrelated active stream")
	default:
	}
}

func TestInjectedDeliveryLeaseLossCancelsOwningStream(t *testing.T) {
	t.Parallel()

	processor, _, _, _, dispatcher := newInjectionRedeliveryFixture(t)
	dispatcher.MarkActive("route")
	streamCtx, cancelStream := context.WithCancel(context.Background())
	processor.activeStreams.Store(deliveryBotID+":route", cancelStream)

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	value, loaded := processor.inflightEventDelivery.Load(deliveryEventID)
	if !loaded {
		t.Fatal("injected delivery lease missing")
	}
	lease := value.(*pipelinepkg.EventDeliveryLease)
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("release injected lease: %v", err)
	}
	select {
	case <-streamCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("owning stream was not canceled after injected delivery lease loss")
	}
	processor.drainQueue(deliveryContext(), "route")
}

func waitForEventDeliveryRelease(t *testing.T, processor *ChannelInboundProcessor, eventID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if _, loaded := processor.inflightEventDelivery.Load(eventID); !loaded {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("event delivery %s remained in flight", eventID)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForDurableEventDeliveryRelease(t *testing.T, queries *durableEventQueries) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		queries.mu.Lock()
		claimed := queries.claimToken.Valid
		queries.mu.Unlock()
		if !claimed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("durable event delivery claim remained in flight")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForDeliveryCompletion(
	t *testing.T,
	processor *ChannelInboundProcessor,
	queries *durableEventQueries,
	eventID string,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		_, loaded := processor.inflightEventDelivery.Load(eventID)
		if !loaded && queries.deliveryIsCompleted() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("event delivery %s did not complete", eventID)
		}
		time.Sleep(time.Millisecond)
	}
}
