package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

func (q *durableEventQueries) NextSessionEventCursor(context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cursor < 100 {
		q.cursor = 100
	}
	q.cursor++
	return q.cursor, nil
}

func (q *durableEventQueries) ClaimSessionEventDelivery(_ context.Context, arg dbsqlc.ClaimSessionEventDeliveryParams) (pgtype.Timestamptz, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.deliveryCompleted {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	if q.claimToken.Valid && q.claimToken != arg.ClaimToken && q.claimedUntil.After(time.Now()) {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	if q.responseOnClaim {
		q.response = true
		q.responseOnClaim = false
	}
	if q.historyResponseOnClaim {
		q.history = true
		q.historyID = deliveryPGUUID("44444444-4444-4444-4444-444444444444")
		q.response = true
		q.historyResponseOnClaim = false
	}
	q.claimToken = arg.ClaimToken
	q.claimedUntil = time.Now().Add(time.Duration(arg.LeaseMs) * time.Millisecond)
	return pgtype.Timestamptz{Time: q.claimedUntil, Valid: true}, nil
}

func (q *durableEventQueries) CompleteSessionEventDelivery(_ context.Context, arg dbsqlc.CompleteSessionEventDeliveryParams) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.completionFailures > 0 {
		q.completionFailures--
		return 0, errors.New("delivery completion unavailable")
	}
	if !q.claimToken.Valid || q.claimToken != arg.ClaimToken || !q.history || (q.pending && !q.response) {
		return 0, nil
	}
	q.deliveryCompleted = true
	q.claimToken = pgtype.UUID{}
	q.claimedUntil = time.Time{}
	return 1, nil
}

func (q *durableEventQueries) RenewSessionEventDelivery(_ context.Context, arg dbsqlc.RenewSessionEventDeliveryParams) (pgtype.Timestamptz, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.claimToken.Valid || q.claimToken != arg.ClaimToken {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	q.claimedUntil = time.Now().Add(time.Duration(arg.LeaseMs) * time.Millisecond)
	return pgtype.Timestamptz{Time: q.claimedUntil, Valid: true}, nil
}

func (q *durableEventQueries) ReleaseSessionEventDelivery(_ context.Context, arg dbsqlc.ReleaseSessionEventDeliveryParams) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.claimToken.Valid || q.claimToken != arg.ClaimToken {
		return 0, nil
	}
	q.claimToken = pgtype.UUID{}
	q.claimedUntil = time.Time{}
	return 1, nil
}

func (q *durableEventQueries) IsSessionEventDeliveryCompleted(context.Context, pgtype.UUID) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.event == nil {
		return false, pgx.ErrNoRows
	}
	return q.deliveryCompleted, nil
}

func (q *durableEventQueries) expireDeliveryClaim() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claimedUntil = time.Now().Add(-time.Second)
}

func (q *durableEventQueries) markDeliveryHandled() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.history = true
	q.historyID = deliveryPGUUID("44444444-4444-4444-4444-444444444444")
	q.pending = false
}

func (q *durableEventQueries) deliveryIsCompleted() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.deliveryCompleted
}

func (w *durableHistoryWriter) ListVisibleTurnResponsesByRequest(context.Context, string, string) ([]messagepkg.Message, error) {
	w.replayCalls++
	return append([]messagepkg.Message(nil), w.replayMessages...), nil
}

func TestDiscussEventDeliveryContinuesAfterHistoryOnlyCrash(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	queries.deliveryCompleted = false
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, notifier, _ := newEventDeliveryProcessor(sessionpkg.TypeDiscuss, queries, writer, pipeline)

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if writer.calls != 0 || notifier.calls != 1 {
		t.Fatalf("history-only recovery calls = history:%d notify:%d, want 0/1", writer.calls, notifier.calls)
	}
	if queries.deliveryCompleted {
		t.Fatal("history-only recovery completed before driver terminal")
	}
	completeCapturedDiscussDelivery(t, processor, notifier)
	if !queries.deliveryCompleted {
		t.Fatal("driver terminal did not mark history-only recovery complete")
	}
}

func completeCapturedDiscussDelivery(t *testing.T, processor *ChannelInboundProcessor, notifier *countingDiscussNotifier) {
	t.Helper()
	delivery := notifier.lastConfig.EventDelivery
	if delivery == nil || delivery.Lease == nil {
		t.Fatal("discuss notifier did not receive the event delivery lease")
	}
	if err := delivery.Lease.Complete(context.Background()); err != nil {
		t.Fatalf("complete discuss delivery: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, loaded := processor.inflightEventDelivery.Load(delivery.EventID); !loaded {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("completed discuss delivery remained in the local in-flight map")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNonDiscussRedeliveryReplaysDurableResponseWithoutGateway(t *testing.T) {
	queries := seededDurableEventQueries(t, false)
	queries.history = true
	queries.response = true
	queries.historyID = deliveryPGUUID("44444444-4444-4444-4444-444444444444")
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	replayContent, err := json.Marshal(conversation.ModelMessage{
		Role:    "assistant",
		Content: conversation.NewTextContent("durable reply"),
	})
	if err != nil {
		t.Fatalf("marshal replay message: %v", err)
	}
	writer := &durableHistoryWriter{
		queries:  queries,
		pipeline: pipeline,
		replayMessages: []messagepkg.Message{{
			ID:      "55555555-5555-5555-5555-555555555555",
			Role:    "assistant",
			Content: replayContent,
		}},
	}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	sender := &fakeReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), sender); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if gatewayCalls != 0 {
		t.Fatalf("gateway calls = %d, want durable response replay", gatewayCalls)
	}
	if gateway.pendingInputCalls != 0 {
		t.Fatalf("pending user input lookups = %d, want 0 for text replay", gateway.pendingInputCalls)
	}
	if writer.completionCalls != 0 {
		t.Fatalf("pending delivery completion calls = %d, want 0 for ordinary history", writer.completionCalls)
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "durable reply" {
		t.Fatalf("replayed replies = %#v, want durable reply", sender.sent)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("delivery was not completed after durable response replay")
	}
}

func TestConcurrentNonDiscussDuplicateStartsGatewayOnce(t *testing.T) {
	for _, tt := range []struct {
		name   string
		second func(first *ChannelInboundProcessor, queries *durableEventQueries, gateway flow.Runner) *ChannelInboundProcessor
	}{
		{
			name: "same processor",
			second: func(first *ChannelInboundProcessor, _ *durableEventQueries, _ flow.Runner) *ChannelInboundProcessor {
				return first
			},
		},
		{
			name: "across processors",
			second: func(_ *ChannelInboundProcessor, queries *durableEventQueries, gateway flow.Runner) *ChannelInboundProcessor {
				return newCrossProcessDeliveryProcessor(queries, gateway)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			queries := &durableEventQueries{}
			gateway := &blockingDeliveryGateway{started: make(chan struct{}), release: make(chan struct{}), onSuccess: queries.markDeliveryHandled}
			first := newCrossProcessDeliveryProcessor(queries, gateway)
			second := tt.second(first, queries, gateway)
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(gateway.release) })

			firstDone := make(chan error, 1)
			go func() {
				firstDone <- first.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{})
			}()
			<-gateway.started

			secondDone := make(chan error, 1)
			go func() {
				secondDone <- second.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{})
			}()
			select {
			case err := <-secondDone:
				if !errors.Is(err, ErrEventDeliveryInFlight) {
					t.Fatalf("competing HandleInbound() error = %v, want in-flight", err)
				}
			case <-time.After(time.Second):
				t.Fatal("competing delivery did not return while first lease was active")
			}
			if got := gateway.calls.Load(); got != 1 {
				t.Fatalf("gateway calls while first delivery is active = %d, want 1", got)
			}

			releaseOnce.Do(func() { close(gateway.release) })
			if err := <-firstDone; err == nil {
				t.Fatal("first HandleInbound() error = nil, want provider failure")
			}
			if err := second.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
				t.Fatalf("retry after first delivery failure HandleInbound() error = %v", err)
			}
			if got := gateway.calls.Load(); got != 2 {
				t.Fatalf("gateway calls after released lease = %d, want 2", got)
			}
		})
	}
}

func newCrossProcessDeliveryProcessor(queries *durableEventQueries, gateway flow.Runner) *ChannelInboundProcessor {
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID: deliverySessionID, Type: sessionpkg.TypeChat, Runtime: sessionpkg.RuntimeModel,
	}})
	processor.SetPipeline(pipeline, pipelinepkg.NewEventStore(nil, queries), nil)
	return processor
}
