package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type snapshotEventQueries struct {
	*durableEventQueries
	events          []dbsqlc.BotSessionEvent
	projectableOnly bool
}

func (q *snapshotEventQueries) ListSessionEventsBySession(context.Context, pgtype.UUID) ([]dbsqlc.BotSessionEvent, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.listErr != nil {
		return nil, q.listErr
	}
	if !q.projectableOnly {
		return append([]dbsqlc.BotSessionEvent(nil), q.events...), nil
	}
	projectable := make([]dbsqlc.BotSessionEvent, 0, len(q.events))
	for _, event := range q.events {
		if event.DeliveryCompletedAt.Valid {
			projectable = append(projectable, event)
		}
	}
	return projectable, nil
}

func (q *snapshotEventQueries) setCurrent(row dbsqlc.BotSessionEvent, completed, history bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.event = &row
	q.deliveryCompleted = completed
	q.history = history
	q.pending = false
	q.response = false
	q.claimToken = pgtype.UUID{}
	q.claimedUntil = time.Time{}
	if history {
		q.historyID = deliveryPGUUID("44444444-4444-4444-4444-444444444444")
	} else {
		q.historyID = pgtype.UUID{}
	}
}

func (q *snapshotEventQueries) appendEvent(row dbsqlc.BotSessionEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, row)
}

func TestCompletedRedeliveryRefreshesWarmPipelineFromDurableSnapshot(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(deliverySessionID, pipelinepkg.ServiceEvent{
		SessionID:    deliverySessionID,
		EventID:      "event_id:stale-local",
		Action:       pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: time.Unix(1_699_999_000, 0).UnixMilli(),
		EventCursor:  1,
		NewTitle:     "stale local title",
	})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	gatewayCalls := 0
	gateway.onChat = func(_ conversation.ChatRequest) { gatewayCalls++ }

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	ic, _ := pipeline.GetIC(deliverySessionID)
	if ic.ChatTitle != "stored title" {
		t.Fatalf("warm pipeline title = %q, want durable stored title", ic.ChatTitle)
	}
	if gatewayCalls != 0 {
		t.Fatalf("gateway calls = %d, want completed redelivery to return", gatewayCalls)
	}
}

func TestCompletedRedeliverySnapshotFailureSkipsProjectionAndGateway(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	queries.listErr = errors.New("snapshot unavailable")
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(deliverySessionID, pipelinepkg.ServiceEvent{
		SessionID:    deliverySessionID,
		EventID:      "event_id:stale-local",
		Action:       pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: time.Unix(1_699_999_000, 0).UnixMilli(),
		EventCursor:  1,
		NewTitle:     "stale local title",
	})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	gatewayCalls := 0
	gateway.onChat = func(_ conversation.ChatRequest) { gatewayCalls++ }

	err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{})
	if err == nil {
		t.Fatal("HandleInbound() error = nil, want snapshot failure")
	}
	ic, _ := pipeline.GetIC(deliverySessionID)
	if ic.ChatTitle != "stale local title" {
		t.Fatalf("warm pipeline title = %q, want unchanged local projection", ic.ChatTitle)
	}
	if gatewayCalls != 0 {
		t.Fatalf("gateway calls = %d, want snapshot failure to fail closed", gatewayCalls)
	}
}

func TestCompletedRedeliveryUnparseableSnapshotKeepsWarmProjection(t *testing.T) {
	base := seededDurableEventQueries(t, true)
	badRow := *base.event
	badRow.ID = deliveryPGUUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab")
	badRow.EventKind = "future-event"
	badRow.EventData = []byte(`{"future":true}`)
	queries := &snapshotEventQueries{
		durableEventQueries: base,
		events:              []dbsqlc.BotSessionEvent{*base.event, badRow},
	}
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(deliverySessionID, pipelinepkg.ServiceEvent{
		SessionID: deliverySessionID, EventID: "event_id:stale-local", Action: pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: time.Unix(1_699_999_000, 0).UnixMilli(), EventCursor: 1, NewTitle: "stale local title",
	})
	processor, gateway := newSnapshotDeliveryProcessor(t, queries, pipeline)
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }

	err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{})
	if err == nil {
		t.Error("HandleInbound() error = nil, want unparseable snapshot failure")
	}
	ic, _ := pipeline.GetIC(deliverySessionID)
	if ic.ChatTitle != "stale local title" {
		t.Errorf("warm pipeline title = %q, want unchanged stale local title", ic.ChatTitle)
	}
	if gatewayCalls != 0 {
		t.Errorf("gateway calls = %d, want 0", gatewayCalls)
	}
}

func TestWarmPipelineRefreshesCompletedEditFromSharedDurableSnapshot(t *testing.T) {
	original := pipelinepkg.MessageEvent{
		SessionID:    deliverySessionID,
		MessageID:    "E1",
		ReceivedAtMs: time.Unix(1_700_000_000, 0).UnixMilli(),
		EventCursor:  10,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "original"}},
	}
	edit := pipelinepkg.EditEvent{
		SessionID:    deliverySessionID,
		EventID:      "event_id:edit-E1",
		MessageID:    "E1",
		ReceivedAtMs: time.Unix(1_700_000_100, 0).UnixMilli(),
		EventCursor:  20,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: "edited"}},
	}
	originalRow := snapshotEventRow(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", "E1", original)
	editRow := snapshotEventRow(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", edit.EventID, edit)
	queries := &snapshotEventQueries{
		durableEventQueries: &durableEventQueries{},
		events:              []dbsqlc.BotSessionEvent{originalRow},
	}
	queries.setCurrent(originalRow, true, true)

	pipelineA := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipelineB := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	processorA, _ := newSnapshotDeliveryProcessor(t, queries, pipelineA)
	processorB, gatewayB := newSnapshotDeliveryProcessor(t, queries, pipelineB)
	originalMessage := deliveryChatMessage()
	originalMessage.Message.ID = "E1"
	originalMessage.Message.Text = "original"
	originalMessage.ReceivedAt = time.Unix(1_700_000_000, 0)
	for name, processor := range map[string]*ChannelInboundProcessor{"A": processorA, "B": processorB} {
		if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), originalMessage, &fakeReplySender{}); err != nil {
			t.Fatalf("warm pipeline %s: %v", name, err)
		}
	}

	queries.appendEvent(editRow)
	queries.setCurrent(editRow, false, true)
	editMessage := originalMessage
	editMessage.Message.Text = "edited"
	editMessage.Metadata = map[string]any{"event_type": "edit", "event_id": "edit-E1"}
	editMessage.ReceivedAt = time.Unix(1_700_000_100, 0)
	if err := processorA.HandleInbound(deliveryContext(), deliveryConfig(), editMessage, &fakeReplySender{}); err != nil {
		t.Fatalf("process edit on pipeline A: %v", err)
	}
	gatewayBCalls := 0
	gatewayB.onChat = func(conversation.ChatRequest) { gatewayBCalls++ }
	if err := processorB.HandleInbound(deliveryContext(), deliveryConfig(), editMessage, &fakeReplySender{}); err != nil {
		t.Fatalf("redeliver completed edit on pipeline B: %v", err)
	}
	if got := snapshotMessageText(t, pipelineB, "E1"); got != "edited" {
		t.Fatalf("warm pipeline B message E1 = %q, want edited", got)
	}
	if gatewayBCalls != 0 {
		t.Fatalf("pipeline B gateway calls = %d, want completed edit redelivery to return", gatewayBCalls)
	}
}

func TestPendingDuplicateReplaysSnapshotWithoutDoubleReduce(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	queries.deliveryCompleted = false
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(deliverySessionID, pipelinepkg.ServiceEvent{
		SessionID:    deliverySessionID,
		EventID:      "event_id:delivery-1",
		Action:       pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: time.Unix(1_700_000_000, 0).UnixMilli(),
		EventCursor:  10,
		NewTitle:     "stored title",
	})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, _ := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	assertDeliveryNodeCount(t, pipeline, 1)
}

func TestDiscussSnapshotExcludesForeignOrphanAndAppendsCurrentAfterHistory(t *testing.T) {
	completed := pipelinepkg.MessageEvent{
		SessionID: deliverySessionID, MessageID: "completed", ReceivedAtMs: 100, EventCursor: 10,
		Content: []pipelinepkg.ContentNode{{Type: "text", Text: "completed message"}},
	}
	foreignOrphan := pipelinepkg.MessageEvent{
		SessionID: deliverySessionID, MessageID: "foreign-orphan", ReceivedAtMs: 200, EventCursor: 20,
		Content: []pipelinepkg.ContentNode{{Type: "text", Text: "foreign orphan"}},
	}
	current := pipelinepkg.MessageEvent{
		SessionID: deliverySessionID, MessageID: "current", ReceivedAtMs: 300, EventCursor: 30,
		Content: []pipelinepkg.ContentNode{{Type: "text", Text: "current message"}},
	}
	completedRow := snapshotEventRow(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", completed.MessageID, completed)
	completedRow.DeliveryCompletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	foreignRow := snapshotEventRow(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", foreignOrphan.MessageID, foreignOrphan)
	currentRow := snapshotEventRow(t, deliveryEventID, current.MessageID, current)
	queries := &snapshotEventQueries{
		durableEventQueries: &durableEventQueries{},
		events:              []dbsqlc.BotSessionEvent{completedRow, foreignRow, currentRow},
		projectableOnly:     true,
	}
	queries.setCurrent(currentRow, false, false)

	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	writer := &durableHistoryWriter{queries: queries.durableEventQueries, pipeline: pipeline}
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, &fakeChatGateway{}, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID: deliverySessionID, Type: sessionpkg.TypeDiscuss, Runtime: sessionpkg.RuntimeModel,
	}})
	processor.SetPipeline(pipeline, pipelinepkg.NewEventStore(nil, queries), nil)
	notifier := &countingDiscussNotifier{}
	processor.discussDriver = notifier
	t.Cleanup(processor.Close)

	msg := deliveryChatMessage()
	msg.Message.ID = current.MessageID
	msg.Message.Text = "current message"
	msg.ReceivedAt = time.UnixMilli(current.ReceivedAtMs)
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), msg, &fakeReplySender{}); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if len(writer.projectionNodesAtPersist) != 1 || writer.projectionNodesAtPersist[0] != 0 {
		t.Fatalf("projection nodes at current history persistence = %v, want [0]", writer.projectionNodesAtPersist)
	}
	if notifier.calls != 1 {
		t.Fatalf("discuss notifications = %d, want 1", notifier.calls)
	}
	ic, _ := pipeline.GetIC(deliverySessionID)
	seen := make(map[string]bool)
	for _, node := range ic.Nodes {
		if node.Message != nil {
			seen[node.Message.MessageID] = true
		}
	}
	if !seen[completed.MessageID] || !seen[current.MessageID] || seen[foreignOrphan.MessageID] {
		t.Fatalf("projected messages = %#v, want completed+current without foreign orphan", seen)
	}
}

func TestInFlightDuplicateDoesNotRefreshWarmProjection(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	queries.deliveryCompleted = false
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(deliverySessionID, pipelinepkg.ServiceEvent{
		SessionID:    deliverySessionID,
		EventID:      "event_id:stale-local",
		Action:       pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: time.Unix(1_699_999_000, 0).UnixMilli(),
		EventCursor:  1,
		NewTitle:     "unchanged local title",
	})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, _ := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	lease, err := processor.eventStore.ClaimEventDelivery(deliveryContext(), deliveryEventID)
	if err != nil {
		t.Fatalf("claim competing delivery: %v", err)
	}
	t.Cleanup(func() {
		if releaseErr := lease.Release(context.Background()); releaseErr != nil {
			t.Errorf("release competing delivery: %v", releaseErr)
		}
	})

	err = processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), &fakeReplySender{})
	if !errors.Is(err, ErrEventDeliveryInFlight) {
		t.Fatalf("HandleInbound() error = %v, want in-flight", err)
	}
	ic, _ := pipeline.GetIC(deliverySessionID)
	if ic.ChatTitle != "unchanged local title" {
		t.Fatalf("warm pipeline title = %q, want unchanged local projection", ic.ChatTitle)
	}
}

func TestQueuedReplayRefreshesForeignEventBeforeStartingStream(t *testing.T) {
	queuedMessage := queuedDeliveryMessage()
	queuedEvent := pipelinepkg.MessageEvent{
		SessionID:    deliverySessionID,
		MessageID:    queuedMessage.Message.ID,
		ReceivedAtMs: queuedMessage.ReceivedAt.UnixMilli(),
		EventCursor:  10,
		Content:      []pipelinepkg.ContentNode{{Type: "text", Text: queuedMessage.Message.Text}},
	}
	queuedRow := snapshotEventRow(t, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1", queuedMessage.Message.ID, queuedEvent)
	foreignEvent := pipelinepkg.ServiceEvent{
		SessionID:    deliverySessionID,
		EventID:      "event_id:foreign",
		Action:       pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: queuedMessage.ReceivedAt.Add(time.Second).UnixMilli(),
		EventCursor:  20,
		NewTitle:     "foreign title",
	}
	foreignRow := snapshotEventRow(t, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2", foreignEvent.EventID, foreignEvent)
	queries := &snapshotEventQueries{
		durableEventQueries: &durableEventQueries{},
		events:              []dbsqlc.BotSessionEvent{queuedRow},
	}
	queries.setCurrent(queuedRow, false, false)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	processor, dispatcher, gateway := newSnapshotQueuedProcessor(t, queries, pipeline)
	dispatcher.MarkActive("route")

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), queuedMessage, &fakeReplySender{}); err != nil {
		t.Fatalf("enqueue HandleInbound() error = %v", err)
	}
	assertDeliveryNodeCount(t, pipeline, 1)
	queries.appendEvent(foreignRow)
	projectionNodesAtGateway := 0
	gateway.onChat = func(conversation.ChatRequest) {
		ic, _ := pipeline.GetIC(deliverySessionID)
		projectionNodesAtGateway = len(ic.Nodes)
	}

	processor.drainQueue(deliveryContext(), "route")
	if projectionNodesAtGateway != 2 {
		t.Fatalf("pipeline nodes at queued gateway = %d, want queued + foreign events", projectionNodesAtGateway)
	}
}

func TestCompletedQueuedReplayRefreshesBeforeIdentityResolution(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
	pipeline.PushEvent(deliverySessionID, pipelinepkg.ServiceEvent{
		SessionID: deliverySessionID, EventID: "event_id:stale-local", Action: pipelinepkg.ServiceChatRenamed,
		ReceivedAtMs: 1, EventCursor: 1, NewTitle: "stale local title",
	})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipeline}
	processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	task := QueuedTask{
		Cfg: deliveryConfig(), Msg: queuedDeliveryMessage(), Sender: &fakeReplySender{},
		PersistedUserMessageID: queries.historyID.String(), EventID: deliveryEventID, SessionID: deliverySessionID,
	}

	err := processor.HandleInbound(withQueuedReplayState(context.Background(), task), task.Cfg, task.Msg, task.Sender)
	if err != nil {
		t.Fatalf("completed queued replay error = %v", err)
	}
	ic, _ := pipeline.GetIC(deliverySessionID)
	if ic.ChatTitle != "stored title" {
		t.Fatalf("warm pipeline title = %q, want durable stored title", ic.ChatTitle)
	}
	task.SessionID = ""
	err = processor.HandleInbound(withQueuedReplayState(context.Background(), task), task.Cfg, task.Msg, task.Sender)
	if err == nil || err.Error() != "completed queued replay is missing durable session id" {
		t.Fatalf("completed queued replay without session error = %v, want missing durable session id", err)
	}
	if gatewayCalls != 0 || writer.calls != 0 {
		t.Fatalf("completed queued side effects = gateway:%d history:%d, want 0/0", gatewayCalls, writer.calls)
	}
}

func snapshotEventRow(t *testing.T, rowID, externalMessageID string, event pipelinepkg.CanonicalEvent) dbsqlc.BotSessionEvent {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal snapshot event: %v", err)
	}
	return dbsqlc.BotSessionEvent{
		ID:                deliveryPGUUID(rowID),
		BotID:             deliveryPGUUID(deliveryBotID),
		SessionID:         deliveryPGUUID(deliverySessionID),
		EventKind:         string(event.Kind()),
		EventData:         data,
		ExternalMessageID: pgtype.Text{String: externalMessageID, Valid: externalMessageID != ""},
		ReceivedAtMs:      event.GetReceivedAtMs(),
	}
}

func newSnapshotDeliveryProcessor(t *testing.T, queries *snapshotEventQueries, pipeline *pipelinepkg.Pipeline) (*ChannelInboundProcessor, *fakeChatGateway) {
	t.Helper()
	routes := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat", RouteID: "route"}}
	gateway := &fakeChatGateway{}
	writer := &durableHistoryWriter{queries: queries.durableEventQueries, pipeline: pipeline}
	processor := NewChannelInboundProcessor(slog.Default(), nil, routes, writer, gateway, nil, nil, "", 0)
	processor.SetSessionEnsurer(&fakeSessionEnsurer{activeSession: SessionResult{
		ID: deliverySessionID, Type: sessionpkg.TypeChat, Runtime: sessionpkg.RuntimeModel,
	}})
	processor.SetPipeline(pipeline, pipelinepkg.NewEventStore(nil, queries), nil)
	t.Cleanup(processor.Close)
	return processor, gateway
}

func newSnapshotQueuedProcessor(t *testing.T, queries *snapshotEventQueries, pipeline *pipelinepkg.Pipeline) (*ChannelInboundProcessor, *RouteDispatcher, *fakeChatGateway) {
	t.Helper()
	processor, gateway := newSnapshotDeliveryProcessor(t, queries, pipeline)
	dispatcher := NewRouteDispatcher(slog.Default())
	processor.SetDispatcher(dispatcher)
	return processor, dispatcher, gateway
}

func snapshotMessageText(t *testing.T, pipeline *pipelinepkg.Pipeline, messageID string) string {
	t.Helper()
	ic, ok := pipeline.GetIC(deliverySessionID)
	if !ok {
		t.Fatal("pipeline session was not loaded")
	}
	for _, node := range ic.Nodes {
		if node.Message != nil && node.Message.MessageID == messageID {
			return pipelinepkg.ContentToPlainText(node.Message.Content)
		}
	}
	t.Fatalf("pipeline message %s was not found", messageID)
	return ""
}
