package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// EventStore persists and loads CanonicalEvents from the database.
type EventStore struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewEventStore creates an EventStore.
func NewEventStore(log *slog.Logger, queries *sqlc.Queries) *EventStore {
	if log == nil {
		log = slog.Default()
	}
	return &EventStore{
		queries: queries,
		logger:  log.With(slog.String("service", "pipeline_event_store")),
	}
}

// PersistEvent writes a CanonicalEvent to the bot_session_events table.
func (s *EventStore) PersistEvent(ctx context.Context, botID, sessionID string, event CanonicalEvent) error {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return fmt.Errorf("invalid bot id: %w", err)
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session id: %w", err)
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
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

	if err = s.queries.CreateSessionEvent(ctx, sqlc.CreateSessionEventParams{
		BotID:                   pgBotID,
		SessionID:               pgSessionID,
		EventKind:               string(event.Kind()),
		EventData:               eventData,
		ExternalMessageID:       pgExternalMsgID,
		SenderChannelIdentityID: pgSenderID,
		ReceivedAtMs:            event.GetReceivedAtMs(),
	}); err != nil {
		return fmt.Errorf("persist session event: %w", err)
	}

	return nil
}

// LoadEvents loads all events for a session, ordered by received_at_ms.
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
			s.logger.Warn("skip unparseable event",
				slog.String("session_id", sessionID),
				slog.String("event_id", row.ID.String()),
				slog.Any("error", parseErr))
			continue
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
		return strings.TrimSpace(e.MessageID)
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
