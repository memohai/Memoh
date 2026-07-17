package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

// EventStore persists and loads CanonicalEvents from the database.
type EventStore struct {
	queries               dbstore.Queries
	logger                *slog.Logger
	deliveryLeaseDuration time.Duration
	deliveryRenewInterval time.Duration
}

type sessionEventDeliveryStateReader interface {
	GetSessionEventDeliveryState(context.Context, pgtype.UUID) (sqlc.GetSessionEventDeliveryStateRow, error)
}

type sessionEventIdentityReader interface {
	GetSessionEventIDByIdentity(context.Context, sqlc.GetSessionEventIDByIdentityParams) (pgtype.UUID, error)
}

type sessionEventDeliveryCompletionReader interface {
	IsSessionEventDeliveryCompleted(context.Context, pgtype.UUID) (bool, error)
}

type sessionEventCursorAllocator interface {
	NextSessionEventCursor(context.Context) (int64, error)
}

// PersistEventResult describes the durable event row used by downstream
// projection and history persistence.
type PersistEventResult struct {
	ID                      string
	Event                   CanonicalEvent
	Inserted                bool
	HistoryMessageID        string
	HistoryDeliveryPending  bool
	HistoryPersisted        bool
	ResponsePersisted       bool
	ReplayResponsePersisted bool
	DeliveryCompleted       bool
}

// NewEventStore creates an EventStore.
func NewEventStore(log *slog.Logger, queries dbstore.Queries) *EventStore {
	if log == nil {
		log = slog.Default()
	}
	return &EventStore{
		queries:               queries,
		logger:                log.With(slog.String("service", "pipeline_event_store")),
		deliveryLeaseDuration: 2 * time.Minute,
		deliveryRenewInterval: 30 * time.Second,
	}
}

func (s *EventStore) IsEventDeliveryCompleted(ctx context.Context, eventID string) (bool, error) {
	reader, ok := s.queries.(sessionEventDeliveryCompletionReader)
	if !ok {
		return false, errors.New("session event delivery completion reader is not configured")
	}
	pgEventID, err := dbpkg.ParseUUID(eventID)
	if err != nil {
		return false, fmt.Errorf("invalid event id: %w", err)
	}
	completed, err := reader.IsSessionEventDeliveryCompleted(ctx, pgEventID)
	if err != nil {
		return false, fmt.Errorf("read session event delivery completion: %w", err)
	}
	return completed, nil
}

func (s *EventStore) LoadEventDeliveryState(ctx context.Context, eventID string) (PersistEventResult, error) {
	pgEventID, err := dbpkg.ParseUUID(eventID)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("invalid event id: %w", err)
	}
	result, err := s.loadEventDeliveryState(ctx, pgEventID)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("load event delivery state: %w", err)
	}
	return result, nil
}

// PersistEvent writes a CanonicalEvent to the bot_session_events table.
func (s *EventStore) PersistEvent(ctx context.Context, botID, sessionID string, event CanonicalEvent) (PersistEventResult, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("invalid bot id: %w", err)
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("invalid session id: %w", err)
	}

	allocator, ok := s.queries.(sessionEventCursorAllocator)
	if !ok {
		return PersistEventResult{}, errors.New("session event cursor allocator is not configured")
	}
	cursor, err := allocator.NextSessionEventCursor(ctx)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("allocate event cursor: %w", err)
	}
	event, err = assignEventCursor(event, cursor)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("assign event cursor: %w", err)
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("marshal event data: %w", err)
	}

	externalMessageID := extractExternalMessageID(event)
	senderID := extractSenderChannelIdentityID(event)

	pgExternalMsgID := pgtype.Text{}
	if externalMessageID != "" {
		pgExternalMsgID = pgtype.Text{String: externalMessageID, Valid: true}
	}

	pgSenderID := pgtype.UUID{}
	if senderID != "" {
		if parsed, parseErr := dbpkg.ParseUUID(senderID); parseErr == nil {
			pgSenderID = parsed
		}
	}

	pgID, err := s.queries.CreateSessionEvent(ctx, sqlc.CreateSessionEventParams{
		BotID:                   pgBotID,
		SessionID:               pgSessionID,
		EventKind:               string(event.Kind()),
		EventData:               eventData,
		ExternalMessageID:       pgExternalMsgID,
		SenderChannelIdentityID: pgSenderID,
		ReceivedAtMs:            event.GetReceivedAtMs(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if externalMessageID == "" {
				return PersistEventResult{}, errors.New("duplicate session event has no stable delivery identity")
			}
			identityReader, ok := s.queries.(sessionEventIdentityReader)
			if !ok {
				return PersistEventResult{}, errors.New("session event identity reader is not configured")
			}
			storedEventID, loadErr := identityReader.GetSessionEventIDByIdentity(ctx, sqlc.GetSessionEventIDByIdentityParams{
				SessionID:         pgSessionID,
				EventKind:         string(event.Kind()),
				ExternalMessageID: externalMessageID,
			})
			if loadErr != nil {
				return PersistEventResult{}, fmt.Errorf("resolve duplicate session event: %w", loadErr)
			}
			result, loadErr := s.loadEventDeliveryState(ctx, storedEventID)
			if loadErr != nil {
				return PersistEventResult{}, fmt.Errorf("load duplicate session event: %w", loadErr)
			}
			return result, nil
		}
		return PersistEventResult{}, fmt.Errorf("persist session event: %w", err)
	}

	if pgID.Valid {
		return PersistEventResult{ID: pgID.String(), Event: event, Inserted: true}, nil
	}
	return PersistEventResult{}, errors.New("persist session event returned invalid id")
}

func (s *EventStore) loadEventDeliveryState(ctx context.Context, eventID pgtype.UUID) (PersistEventResult, error) {
	reader, ok := s.queries.(sessionEventDeliveryStateReader)
	if !ok {
		return PersistEventResult{}, errors.New("session event delivery state reader is not configured")
	}
	row, err := reader.GetSessionEventDeliveryState(ctx, eventID)
	if err != nil {
		return PersistEventResult{}, err
	}
	if !row.ID.Valid || row.ID != eventID {
		return PersistEventResult{}, errors.New("session event delivery state has invalid id")
	}
	storedEvent, err := parseEventData(row.EventKind, row.EventData)
	if err != nil {
		return PersistEventResult{}, fmt.Errorf("parse session event: %w", err)
	}
	return PersistEventResult{
		ID:                      row.ID.String(),
		Event:                   storedEvent,
		HistoryMessageID:        row.HistoryMessageID.String(),
		HistoryDeliveryPending:  row.HistoryDeliveryPending,
		HistoryPersisted:        row.HistoryPersisted,
		ResponsePersisted:       row.ResponsePersisted,
		ReplayResponsePersisted: row.ReplayResponsePersisted,
		DeliveryCompleted:       row.DeliveryCompleted,
	}, nil
}

// LoadEvents loads completed or history-ready events for a session, ordered by received_at_ms.
// Callers add the event whose delivery lease they currently own before projection.
func (s *EventStore) LoadEvents(ctx context.Context, sessionID string) ([]CanonicalEvent, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid session id: %w", err)
	}

	rows, err := s.queries.ListSessionEventsBySession(ctx, pgSessionID)
	if err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}

	events := make([]CanonicalEvent, 0, len(rows))
	for _, row := range rows {
		event, parseErr := parseEventData(row.EventKind, row.EventData)
		if parseErr != nil {
			return nil, fmt.Errorf("parse session event %s (%s): %w", row.ID.String(), row.EventKind, parseErr)
		}
		events = append(events, event)
	}

	return events, nil
}

// HasEvents checks whether a session has any events persisted.
func (s *EventStore) HasEvents(ctx context.Context, sessionID string) (bool, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return false, fmt.Errorf("invalid session id: %w", err)
	}

	count, err := s.queries.CountSessionEvents(ctx, pgSessionID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *EventStore) GetDiscussCursor(ctx context.Context, sessionID, scopeKey string) (DiscussCursorPosition, error) {
	if s == nil || s.queries == nil {
		return DiscussCursorPosition{}, nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return DiscussCursorPosition{}, fmt.Errorf("invalid session id: %w", err)
	}
	row, err := s.queries.GetSessionDiscussCursor(ctx, sqlc.GetSessionDiscussCursorParams{
		SessionID: pgSessionID,
		ScopeKey:  normalizeDiscussCursorScope(scopeKey),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DiscussCursorPosition{}, nil
		}
		return DiscussCursorPosition{}, fmt.Errorf("get discuss cursor: %w", err)
	}
	return DiscussCursorPosition{
		SourceCursor: row.ConsumedCursor,
		EventCursor:  row.ConsumedEventCursor,
	}, nil
}

func (s *EventStore) GetDiscussEventCursorFloor(ctx context.Context, sessionID string) (int64, error) {
	if s == nil || s.queries == nil {
		return 0, nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return 0, fmt.Errorf("invalid session id: %w", err)
	}
	cursor, err := s.queries.GetSessionDiscussEventCursorFloor(ctx, pgSessionID)
	if err != nil {
		return 0, fmt.Errorf("get discuss event cursor floor: %w", err)
	}
	return cursor, nil
}

func (s *EventStore) UpsertDiscussCursor(ctx context.Context, sessionID, scopeKey, routeID, source string, position DiscussCursorPosition) error {
	if s == nil || s.queries == nil || (position.SourceCursor <= 0 && position.EventCursor <= 0) {
		return nil
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session id: %w", err)
	}
	pgRouteID := pgtype.UUID{}
	if strings.TrimSpace(routeID) != "" {
		parsed, parseErr := dbpkg.ParseUUID(routeID)
		if parseErr != nil {
			return fmt.Errorf("invalid route id: %w", parseErr)
		}
		pgRouteID = parsed
	}
	_, err = s.queries.UpsertSessionDiscussCursor(ctx, sqlc.UpsertSessionDiscussCursorParams{
		SessionID:           pgSessionID,
		ScopeKey:            normalizeDiscussCursorScope(scopeKey),
		RouteID:             pgRouteID,
		Source:              strings.TrimSpace(source),
		ConsumedCursor:      position.SourceCursor,
		ConsumedEventCursor: position.EventCursor,
	})
	if err != nil {
		return fmt.Errorf("upsert discuss cursor: %w", err)
	}
	return nil
}

func normalizeDiscussCursorScope(scopeKey string) string {
	if strings.TrimSpace(scopeKey) == "" {
		return "default"
	}
	return strings.TrimSpace(scopeKey)
}

func parseEventData(kind string, data []byte) (CanonicalEvent, error) {
	switch EventKind(kind) {
	case EventMessage:
		var e MessageEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventEdit:
		var e EditEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventDelete:
		var e DeleteEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventService:
		var e ServiceEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	default:
		return nil, fmt.Errorf("unknown event kind: %s", kind)
	}
}

func extractExternalMessageID(event CanonicalEvent) string {
	switch e := event.(type) {
	case MessageEvent:
		return strings.TrimSpace(e.MessageID)
	case EditEvent:
		return strings.TrimSpace(e.EventID)
	case DeleteEvent:
		return strings.TrimSpace(e.EventID)
	case ServiceEvent:
		return strings.TrimSpace(e.EventID)
	default:
		return ""
	}
}

func extractSenderChannelIdentityID(event CanonicalEvent) string {
	switch e := event.(type) {
	case MessageEvent:
		if e.Sender != nil {
			return strings.TrimSpace(e.Sender.ID)
		}
	case EditEvent:
		if e.Sender != nil {
			return strings.TrimSpace(e.Sender.ID)
		}
	case ServiceEvent:
		if e.Actor != nil {
			return strings.TrimSpace(e.Actor.ID)
		}
	}
	return ""
}
