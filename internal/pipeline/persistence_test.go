package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type persistenceQueries struct {
	dbstore.Queries
	createdID     pgtype.UUID
	createErr     error
	createArg     sqlc.CreateSessionEventParams
	nextCursor    int64
	cursorErr     error
	deliveryState sqlc.GetSessionEventDeliveryStateRow
	deliveryErr   error
	identityArg   sqlc.GetSessionEventIDByIdentityParams
	stateEventID  pgtype.UUID
}

func (q *persistenceQueries) NextSessionEventCursor(context.Context) (int64, error) {
	return q.nextCursor, q.cursorErr
}

func (q *persistenceQueries) CreateSessionEvent(_ context.Context, arg sqlc.CreateSessionEventParams) (pgtype.UUID, error) {
	q.createArg = arg
	return q.createdID, q.createErr
}

func (q *persistenceQueries) GetSessionEventIDByIdentity(_ context.Context, arg sqlc.GetSessionEventIDByIdentityParams) (pgtype.UUID, error) {
	q.identityArg = arg
	return q.deliveryState.ID, nil
}

func (q *persistenceQueries) GetSessionEventDeliveryState(_ context.Context, eventID pgtype.UUID) (sqlc.GetSessionEventDeliveryStateRow, error) {
	q.stateEventID = eventID
	return q.deliveryState, q.deliveryErr
}

func TestPersistEventReturnsInsertedCanonicalEvent(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		eventID   = "33333333-3333-3333-3333-333333333333"
	)
	queries := &persistenceQueries{nextCursor: 41, createdID: pgtype.UUID{
		Bytes: [16]byte{0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33},
		Valid: true,
	}}
	store := NewEventStore(nil, queries)
	event := ServiceEvent{
		SessionID:    sessionID,
		EventID:      "event_id:delivery-1",
		Action:       ServiceChatRenamed,
		ReceivedAtMs: 100,
		EventCursor:  7,
		NewTitle:     "new title",
	}

	got, err := store.PersistEvent(context.Background(), botID, sessionID, event)
	if err != nil {
		t.Fatalf("PersistEvent() error = %v", err)
	}
	if got.ID != eventID || !got.Inserted || got.HistoryPersisted {
		t.Fatalf("PersistEvent() = %#v, want inserted event %s without history", got, eventID)
	}
	stored, ok := got.Event.(ServiceEvent)
	if !ok || stored.EventCursor != 41 || stored.NewTitle != "new title" {
		t.Fatalf("stored event = %#v, want database cursor 41", got.Event)
	}
	var persisted ServiceEvent
	if err := json.Unmarshal(queries.createArg.EventData, &persisted); err != nil {
		t.Fatalf("unmarshal persisted event: %v", err)
	}
	if persisted.EventCursor != 41 {
		t.Fatalf("persisted event cursor = %d, want 41", persisted.EventCursor)
	}
}

func TestPersistEventReturnsOriginalDuplicateState(t *testing.T) {
	t.Parallel()

	const (
		botID            = "11111111-1111-1111-1111-111111111111"
		sessionID        = "22222222-2222-2222-2222-222222222222"
		eventID          = "33333333-3333-3333-3333-333333333333"
		historyMessageID = "44444444-4444-4444-4444-444444444444"
	)
	stored := ServiceEvent{
		SessionID:    sessionID,
		EventID:      "event_id:delivery-1",
		Action:       ServiceChatRenamed,
		ReceivedAtMs: 100,
		EventCursor:  7,
		NewTitle:     "stored title",
	}
	storedData, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal stored event: %v", err)
	}
	queries := &persistenceQueries{
		nextCursor: 99,
		createErr:  pgx.ErrNoRows,
		deliveryState: sqlc.GetSessionEventDeliveryStateRow{
			ID:                      pgtype.UUID{Bytes: [16]byte{0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33}, Valid: true},
			EventKind:               string(EventService),
			EventData:               storedData,
			HistoryMessageID:        pgtype.UUID{Bytes: [16]byte{0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44}, Valid: true},
			HistoryPersisted:        true,
			ResponsePersisted:       true,
			ReplayResponsePersisted: true,
			DeliveryCompleted:       true,
		},
	}
	store := NewEventStore(nil, queries)
	redelivery := stored
	redelivery.EventCursor = 99
	redelivery.NewTitle = "redelivered title"

	got, err := store.PersistEvent(context.Background(), botID, sessionID, redelivery)
	if err != nil {
		t.Fatalf("PersistEvent() error = %v", err)
	}
	if got.ID != eventID || got.HistoryMessageID != historyMessageID || got.Inserted || !got.HistoryPersisted || !got.DeliveryCompleted {
		t.Fatalf("PersistEvent() = %#v, want completed duplicate %s", got, eventID)
	}
	if !got.ReplayResponsePersisted || queries.stateEventID.String() != eventID ||
		queries.identityArg.SessionID.String() != sessionID || queries.identityArg.EventKind != string(EventService) {
		t.Fatalf("duplicate delivery lookup/state = %#v/%s/%#v", got, queries.stateEventID.String(), queries.identityArg)
	}
	gotStored, ok := got.Event.(ServiceEvent)
	if !ok || gotStored.EventCursor != 7 || gotStored.NewTitle != "stored title" {
		t.Fatalf("duplicate event = %#v, want original persisted event", got.Event)
	}
}

func TestPersistEventDoesNotCompleteDuplicateFromHistoryAlone(t *testing.T) {
	t.Parallel()

	stored := ServiceEvent{
		SessionID: "22222222-2222-2222-2222-222222222222", EventID: "event_id:delivery-1",
		Action: ServiceChatRenamed, EventCursor: 7,
	}
	storedData, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal stored event: %v", err)
	}
	queries := &persistenceQueries{
		nextCursor: 99,
		createErr:  pgx.ErrNoRows,
		deliveryState: sqlc.GetSessionEventDeliveryStateRow{
			ID: pgtype.UUID{Bytes: [16]byte{0x33}, Valid: true}, EventKind: string(EventService), EventData: storedData,
			HistoryMessageID: pgtype.UUID{Bytes: [16]byte{0x44}, Valid: true}, HistoryPersisted: true,
		},
	}
	got, err := NewEventStore(nil, queries).PersistEvent(context.Background(),
		"11111111-1111-1111-1111-111111111111", stored.SessionID, stored)
	if err != nil {
		t.Fatalf("PersistEvent() error = %v", err)
	}
	if !got.HistoryPersisted || got.DeliveryCompleted {
		t.Fatalf("duplicate state = history:%t complete:%t, want true/false", got.HistoryPersisted, got.DeliveryCompleted)
	}
}

func TestPersistEventFailsClosedWhenCursorAllocationFails(t *testing.T) {
	t.Parallel()

	queries := &persistenceQueries{cursorErr: errors.New("cursor unavailable")}
	store := NewEventStore(nil, queries)
	_, err := store.PersistEvent(context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		ServiceEvent{SessionID: "22222222-2222-2222-2222-222222222222", EventID: "event_id:delivery-1", Action: ServiceChatRenamed},
	)
	if err == nil || !strings.Contains(err.Error(), "allocate event cursor") {
		t.Fatalf("PersistEvent() error = %v, want cursor allocation failure", err)
	}
	if queries.createArg.EventKind != "" {
		t.Fatal("event persistence ran after cursor allocation failure")
	}
}

func TestPersistEventFailsClosedWhenDuplicateStateCannotLoad(t *testing.T) {
	t.Parallel()

	queries := &persistenceQueries{nextCursor: 11, createErr: pgx.ErrNoRows, deliveryErr: errors.New("read failed")}
	store := NewEventStore(nil, queries)
	_, err := store.PersistEvent(context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		ServiceEvent{SessionID: "22222222-2222-2222-2222-222222222222", EventID: "event_id:delivery-1", Action: ServiceChatRenamed},
	)
	if err == nil || !strings.Contains(err.Error(), "load duplicate session event") {
		t.Fatalf("PersistEvent() error = %v, want duplicate lookup failure", err)
	}
}

func TestExtractExternalMessageIDDeduplicatesStableEventDeliveries(t *testing.T) {
	t.Parallel()

	if got := extractExternalMessageID(MessageEvent{MessageID: "message"}); got != "message" {
		t.Fatalf("message external id = %q, want message", got)
	}
	if got := extractExternalMessageID(EditEvent{MessageID: "message", EventID: "edit-1"}); got != "edit-1" {
		t.Fatalf("edit external id = %q, want stable delivery id", got)
	}
	if got := extractExternalMessageID(EditEvent{MessageID: "message"}); got != "" {
		t.Fatalf("edit without delivery identity = %q, want empty", got)
	}
	if got := extractExternalMessageID(ServiceEvent{EventID: "service-1"}); got != "service-1" {
		t.Fatalf("service external id = %q, want stable delivery id", got)
	}
}
