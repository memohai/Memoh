package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

const (
	deliveryBotID     = "11111111-1111-1111-1111-111111111111"
	deliverySessionID = "22222222-2222-2222-2222-222222222222"
	deliveryEventID   = "33333333-3333-3333-3333-333333333333"
)

type durableEventQueries struct {
	dbstore.Queries

	mu                     sync.Mutex
	event                  *dbsqlc.BotSessionEvent
	history                bool
	historyID              pgtype.UUID
	pending                bool
	response               bool
	responseCoveredOnly    bool
	responseOnClaim        bool
	historyResponseOnClaim bool
	completionFailures     int
	deliveryCompleted      bool
	cursor                 int64
	claimToken             pgtype.UUID
	claimedUntil           time.Time
	createErr              error
	listErr                error
}

func TestCheckedEventDeliveryClaimRejectsMismatchedEventWithoutLeakingToken(t *testing.T) {
	t.Parallel()

	const secret = "sensitive-claim-token"
	claim, err := checkedEventDeliveryClaim(deliveryEventID, pipelinepkg.DeliveryClaim{
		EventID:    "99999999-9999-9999-9999-999999999999",
		ClaimToken: secret,
	})
	if err == nil || claim != nil {
		t.Fatalf("checkedEventDeliveryClaim() = %#v, %v, want mismatch", claim, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("event mismatch leaked claim token: %v", err)
	}
}

func (q *durableEventQueries) CreateSessionEvent(_ context.Context, arg dbsqlc.CreateSessionEventParams) (pgtype.UUID, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.createErr != nil {
		return pgtype.UUID{}, q.createErr
	}
	if q.event != nil {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	id := deliveryPGUUID(deliveryEventID)
	q.event = &dbsqlc.BotSessionEvent{
		ID:                      id,
		BotID:                   arg.BotID,
		SessionID:               arg.SessionID,
		EventKind:               arg.EventKind,
		EventData:               append([]byte(nil), arg.EventData...),
		ExternalMessageID:       arg.ExternalMessageID,
		SenderChannelIdentityID: arg.SenderChannelIdentityID,
		ReceivedAtMs:            arg.ReceivedAtMs,
	}
	return id, nil
}

func (q *durableEventQueries) GetSessionEventIDByIdentity(_ context.Context, arg dbsqlc.GetSessionEventIDByIdentityParams) (pgtype.UUID, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.event == nil || q.event.SessionID != arg.SessionID || q.event.EventKind != arg.EventKind || q.event.ExternalMessageID.String != arg.ExternalMessageID {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	return q.event.ID, nil
}

func (q *durableEventQueries) GetSessionEventDeliveryState(_ context.Context, eventID pgtype.UUID) (dbsqlc.GetSessionEventDeliveryStateRow, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.event == nil || q.event.ID != eventID {
		return dbsqlc.GetSessionEventDeliveryStateRow{}, pgx.ErrNoRows
	}
	return dbsqlc.GetSessionEventDeliveryStateRow{
		ID:                      q.event.ID,
		EventKind:               q.event.EventKind,
		EventData:               append([]byte(nil), q.event.EventData...),
		DeliveryCompleted:       q.deliveryCompleted,
		HistoryMessageID:        q.historyID,
		HistoryDeliveryPending:  q.pending,
		HistoryPersisted:        q.history && (!q.pending || q.response),
		ResponsePersisted:       q.response,
		ReplayResponsePersisted: q.response && !q.responseCoveredOnly,
	}, nil
}

func (q *durableEventQueries) ListSessionEventsBySession(context.Context, pgtype.UUID) ([]dbsqlc.BotSessionEvent, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.listErr != nil {
		return nil, q.listErr
	}
	if q.event == nil {
		return nil, nil
	}
	return []dbsqlc.BotSessionEvent{*q.event}, nil
}

func (*durableEventQueries) GetSessionDiscussEventCursorFloor(context.Context, pgtype.UUID) (int64, error) {
	return 0, nil
}

func (q *durableEventQueries) markHistoryPersisted(input messagepkg.PersistInput) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.history {
		return false
	}
	q.history = true
	q.historyID = deliveryPGUUID("44444444-4444-4444-4444-444444444444")
	q.pending = input.Metadata["pipeline_delivery_state"] == "pending"
	return true
}

func (q *durableEventQueries) pendingHistoryState() (bool, string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.pending, q.historyID.String()
}

type durableHistoryWriter struct {
	queries                  *durableEventQueries
	pipeline                 *pipelinepkg.Pipeline
	replayMessages           []messagepkg.Message
	failures                 int
	calls                    int
	completionCalls          int
	replayCalls              int
	projectionNodesAtPersist []int
}

func (w *durableHistoryWriter) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	w.calls++
	ic, _ := w.pipeline.GetIC(input.SessionID)
	w.projectionNodesAtPersist = append(w.projectionNodesAtPersist, len(ic.Nodes))
	if w.failures > 0 {
		w.failures--
		return messagepkg.Message{}, errors.New("history unavailable")
	}
	if !w.queries.markHistoryPersisted(input) {
		return messagepkg.Message{}, messagepkg.ErrEventAlreadyPersisted
	}
	return messagepkg.Message{ID: "44444444-4444-4444-4444-444444444444"}, nil
}

func (w *durableHistoryWriter) CompletePendingDelivery(_ context.Context, messageID string) error {
	w.completionCalls++
	pending, historyID := w.queries.pendingHistoryState()
	if messageID != historyID {
		return errors.New("unexpected pending delivery message id")
	}
	if !pending {
		return nil
	}
	w.queries.mu.Lock()
	w.queries.pending = false
	w.queries.mu.Unlock()
	return nil
}

type countingDiscussNotifier struct {
	calls      int
	lastConfig pipelinepkg.DiscussSessionConfig
}

func (n *countingDiscussNotifier) NotifyRC(_ context.Context, _ string, _ pipelinepkg.RenderedContext, config pipelinepkg.DiscussSessionConfig) {
	n.calls++
	n.lastConfig = config
}

type blockingDeliveryGateway struct {
	fakeChatGateway
	calls     atomic.Int32
	started   chan struct{}
	release   chan struct{}
	onSuccess func()
}

func (g *blockingDeliveryGateway) StreamChat(context.Context, conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	call := g.calls.Add(1)
	if call == 1 {
		close(g.started)
		<-g.release
	}
	chunks := make(chan conversation.StreamChunk)
	errs := make(chan error, 1)
	if call == 1 {
		errs <- errors.New("provider failed")
	} else if g.onSuccess != nil {
		g.onSuccess()
	}
	close(chunks)
	close(errs)
	return chunks, errs
}

func TestDiscussEventDeliveryRepairsOrphanBeforeProjection(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline, failures: 1}
	processor, notifier, _ := newEventDeliveryProcessor(sessionpkg.TypeDiscuss, queries, writer, pipeline)

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err == nil {
		t.Fatal("first HandleInbound() error = nil, want history failure")
	}
	assertDeliveryNodeCount(t, pipeline, 0)
	if notifier.calls != 0 {
		t.Fatalf("notifications after failed history = %d, want 0", notifier.calls)
	}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("retry HandleInbound() error = %v", err)
	}
	assertDeliveryNodeCount(t, pipeline, 1)
	if notifier.calls != 1 {
		t.Fatalf("notifications after repair = %d, want 1", notifier.calls)
	}
	if len(writer.projectionNodesAtPersist) != 2 || writer.projectionNodesAtPersist[0] != 0 || writer.projectionNodesAtPersist[1] != 0 {
		t.Fatalf("projection nodes at history persistence = %v, want [0 0]", writer.projectionNodesAtPersist)
	}
	if queries.deliveryCompleted {
		t.Fatal("discuss delivery completed before driver terminal")
	}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); !errors.Is(err, ErrEventDeliveryInFlight) {
		t.Fatalf("in-flight duplicate HandleInbound() error = %v, want in-flight", err)
	}
	completeCapturedDiscussDelivery(t, processor, notifier)

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("completed duplicate HandleInbound() error = %v", err)
	}
	assertDeliveryNodeCount(t, pipeline, 1)
	if writer.calls != 2 || notifier.calls != 1 {
		t.Fatalf("completed duplicate calls = history:%d notify:%d, want 2/1", writer.calls, notifier.calls)
	}
}

func TestDiscussColdReplayDoesNotProjectOrphanTwice(t *testing.T) {
	queries := seededDurableEventQueries(t, false)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, notifier, _ := newEventDeliveryProcessor(sessionpkg.TypeDiscuss, queries, writer, pipeline)

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	assertDeliveryNodeCount(t, pipeline, 1)
	if writer.calls != 1 || notifier.calls != 1 {
		t.Fatalf("cold repair calls = history:%d notify:%d, want 1/1", writer.calls, notifier.calls)
	}
	completeCapturedDiscussDelivery(t, processor, notifier)
}

func TestEventDeliveryDatabaseErrorsFailClosed(t *testing.T) {
	for _, tt := range []struct {
		name    string
		queries *durableEventQueries
	}{
		{name: "replay", queries: &durableEventQueries{listErr: errors.New("replay unavailable")}},
		{name: "persist", queries: &durableEventQueries{createErr: errors.New("persist unavailable")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
			writer := &durableHistoryWriter{queries: tt.queries, pipeline: pipeline}
			processor, notifier, _ := newEventDeliveryProcessor(sessionpkg.TypeDiscuss, tt.queries, writer, pipeline)

			if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err == nil {
				t.Fatal("HandleInbound() error = nil, want database failure")
			}
			if writer.calls != 0 || notifier.calls != 0 {
				t.Fatalf("failed delivery calls = history:%d notify:%d, want 0/0", writer.calls, notifier.calls)
			}
			assertDeliveryNodeCount(t, pipeline, 0)
		})
	}
}

func TestNonDiscussChatRequestCarriesCurrentDeliveryClaim(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	var wantClaim pipelinepkg.DeliveryClaim
	var claimAvailable bool
	gateway.onChat = func(conversation.ChatRequest) {
		value, loaded := processor.inflightEventDelivery.Load(deliveryEventID)
		if loaded {
			wantClaim, claimAvailable = value.(*pipelinepkg.EventDeliveryLease).DeliveryClaim()
		}
		queries.markDeliveryHandled()
	}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	gotClaim := gateway.gotReq.EventDeliveryClaim
	if !claimAvailable || gotClaim == nil ||
		gotClaim.EventID != wantClaim.EventID ||
		gotClaim.ClaimToken != wantClaim.ClaimToken {
		t.Fatalf("ChatRequest delivery claim = %#v, want %#v", gotClaim, wantClaim)
	}
}

func TestCompletedNonDiscussEventSkipsProjectionAndGateway(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	assertDeliveryNodeCount(t, pipeline, 1)
	if writer.calls != 0 || gatewayCalls != 0 {
		t.Fatalf("completed duplicate calls = history:%d gateway:%d, want 0/0", writer.calls, gatewayCalls)
	}
}

func TestConcurrentNonDiscussDuplicateStartsGatewayOnceAndReleasesAfterFailure(t *testing.T) {
	queries := &durableEventQueries{}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	gateway := &blockingDeliveryGateway{started: make(chan struct{}), release: make(chan struct{}), onSuccess: queries.markDeliveryHandled}
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Type:    sessionpkg.TypeChat,
		Runtime: sessionpkg.RuntimeModel,
	}})
	processor.SetPipeline(pipeline, pipelinepkg.NewEventStore(nil, queries), nil)

	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(gateway.release) })
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{})
	}()
	<-gateway.started

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{})
	}()
	select {
	case err := <-secondDone:
		if !errors.Is(err, ErrEventDeliveryInFlight) {
			t.Fatalf("concurrent duplicate HandleInbound() error = %v, want in-flight", err)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent duplicate did not return while first delivery was in flight")
	}
	if got := gateway.calls.Load(); got != 1 {
		t.Fatalf("gateway calls while first delivery is in flight = %d, want 1", got)
	}

	releaseOnce.Do(func() { close(gateway.release) })
	if err := <-firstDone; err == nil {
		t.Fatal("first HandleInbound() error = nil, want provider failure")
	}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("retry after provider failure HandleInbound() error = %v", err)
	}
	if got := gateway.calls.Load(); got != 2 {
		t.Fatalf("gateway calls after provider retry = %d, want 2", got)
	}
}

func newEventDeliveryProcessor(
	sessionType string,
	queries *durableEventQueries,
	writer messagepkg.Writer,
	pipeline *pipelinepkg.Pipeline,
) (*ChannelInboundProcessor, *countingDiscussNotifier, *fakeChatGateway) {
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID:      deliverySessionID,
		Type:    sessionType,
		Runtime: sessionpkg.RuntimeModel,
	}})
	processor.SetPipeline(pipeline, pipelinepkg.NewEventStore(nil, queries), nil)
	notifier := &countingDiscussNotifier{}
	if sessionType == sessionpkg.TypeDiscuss {
		processor.discussDriver = notifier
	}
	return processor, notifier, gateway
}

func seededDurableEventQueries(t *testing.T, history bool) *durableEventQueries {
	t.Helper()
	event := pipelinepkg.ServiceEvent{
		SessionID:    deliverySessionID,
		EventID:      "event_id:delivery-1",
		Action:       pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: time.Unix(1_700_000_000, 0).UnixMilli(),
		EventCursor:  10,
		NewTitle:     "stored title",
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal seeded event: %v", err)
	}
	queries := &durableEventQueries{
		event: &dbsqlc.BotSessionEvent{
			ID:                deliveryPGUUID(deliveryEventID),
			BotID:             deliveryPGUUID(deliveryBotID),
			SessionID:         deliveryPGUUID(deliverySessionID),
			EventKind:         string(pipelinepkg.EventService),
			EventData:         data,
			ExternalMessageID: pgtype.Text{String: "event_id:delivery-1", Valid: true},
			ReceivedAtMs:      event.ReceivedAtMs,
		},
		history:           history,
		deliveryCompleted: history,
	}
	if history {
		queries.historyID = deliveryPGUUID("44444444-4444-4444-4444-444444444444")
	}
	return queries
}

func deliveryContext() context.Context {
	return WithIdentityState(context.Background(), IdentityState{Identity: InboundIdentity{
		BotID:             deliveryBotID,
		ChannelIdentityID: "55555555-5555-5555-5555-555555555555",
		DisplayName:       "User",
		ForceReply:        true,
	}})
}

func deliveryConfig() channel.ChannelConfig {
	return channel.ChannelConfig{ID: "config", BotID: deliveryBotID, ChannelType: channel.ChannelType("test")}
}

func deliveryMessage() channel.InboundMessage {
	return channel.InboundMessage{
		BotID:       deliveryBotID,
		Channel:     channel.ChannelType("test"),
		Message:     channel.Message{ID: "service-message", Text: "chat renamed"},
		ReplyTarget: "target",
		Sender:      channel.Identity{SubjectID: "sender", DisplayName: "User"},
		Conversation: channel.Conversation{
			ID:   "conversation",
			Type: channel.ConversationTypePrivate,
		},
		Metadata: map[string]any{
			"event_type":     "service",
			"event_id":       "delivery-1",
			"service_action": string(pipelinepkg.ServiceChatRenamed),
			"new_title":      "redelivered title",
		},
		ReceivedAt: time.Unix(1_700_000_100, 0),
	}
}

func deliveryChatMessage() channel.InboundMessage {
	msg := deliveryMessage()
	msg.Metadata = nil
	msg.Message.Text = "hello"
	return msg
}

func assertDeliveryNodeCount(t *testing.T, pipeline *pipelinepkg.Pipeline, want int) {
	t.Helper()
	ic, _ := pipeline.GetIC(deliverySessionID)
	if len(ic.Nodes) != want {
		t.Fatalf("pipeline nodes = %d, want %d", len(ic.Nodes), want)
	}
}

func deliveryPGUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		panic(err)
	}
	return id
}
