package botbackup

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestRestoreHistoryDeduplicatesLegacyMessageEventLinksByMigrationOrder(t *testing.T) {
	ctx := context.Background()
	sourceBotID := newBotBackupTestUUID()
	targetBotID := newBotBackupTestUUID()
	actorUserID := newBotBackupTestUUID()
	sourceSessionID := newBotBackupTestUUID()
	targetSessionID := newBotBackupTestUUID()
	sourceEventID := newBotBackupTestUUID()
	missingEventID := newBotBackupTestUUID()
	targetEventID := newBotBackupTestUUID()
	baseTime := time.Now().UTC().Add(-time.Hour)

	messages := []dbsqlc.ListAllMessagesForBackupRow{
		legacyEventLinkedMessage(
			mustTestPGUUID(t, "00000000-0000-0000-0000-000000000001"),
			sourceBotID,
			sourceSessionID,
			sourceEventID,
			"later",
			baseTime.Add(time.Second),
		),
		legacyEventLinkedMessage(
			mustTestPGUUID(t, "00000000-0000-0000-0000-000000000003"),
			sourceBotID,
			sourceSessionID,
			sourceEventID,
			"winner",
			baseTime,
		),
		legacyEventLinkedMessage(
			mustTestPGUUID(t, "00000000-0000-0000-0000-000000000004"),
			sourceBotID,
			sourceSessionID,
			sourceEventID,
			"same-time-loser",
			baseTime,
		),
		legacyEventLinkedMessage(
			mustTestPGUUID(t, "00000000-0000-0000-0000-000000000005"),
			sourceBotID,
			sourceSessionID,
			missingEventID,
			"missing-event-first",
			baseTime.Add(-2*time.Second),
		),
		legacyEventLinkedMessage(
			mustTestPGUUID(t, "00000000-0000-0000-0000-000000000006"),
			sourceBotID,
			sourceSessionID,
			missingEventID,
			"missing-event-second",
			baseTime.Add(-time.Second),
		),
		legacyEventLinkedMessage(
			mustTestPGUUID(t, "00000000-0000-0000-0000-000000000007"),
			sourceBotID,
			sourceSessionID,
			pgtype.UUID{},
			"post-migration-loser",
			baseTime.Add(2*time.Second),
		),
	}
	messages[5].Metadata = []byte(`{"label":"post-migration-loser","_migration_0115_history_event_dedup":{"version":1,"message_id":"00000000-0000-0000-0000-000000000007","event_id":"00000000-0000-0000-0000-000000000008"}}`)
	queries := &recordingLegacyEventRestoreQueries{
		targetSessionID: targetSessionID,
		targetEventID:   targetEventID,
		linkedEvents:    map[string]struct{}{},
	}
	state := &importState{
		entries: map[string]backupZipEntry{
			"history/sessions.json": jsonBackupEntry(t, []dbsqlc.ListSessionsByBotRow{
				backupRoundTripSession(sourceBotID, sourceSessionID, "legacy"),
			}),
			"history/session_events.json": jsonBackupEntry(t, []dbsqlc.BotSessionEvent{
				{
					ID:        sourceEventID,
					BotID:     sourceBotID,
					SessionID: sourceSessionID,
					EventKind: "message",
					EventData: []byte(`{"event_cursor":1}`),
				},
			}),
			"history/messages.json": jsonBackupEntry(t, messages),
		},
		counts: map[Section]int{},
	}

	svc := &Service{queries: queries}
	if err := svc.restoreHistory(ctx, actorUserID.String(), targetBotID.String(), state, false, false); err != nil {
		t.Fatalf("restoreHistory() error = %v", err)
	}

	if len(queries.createdMessages) != len(messages) {
		t.Fatalf("created messages = %d, want %d", len(queries.createdMessages), len(messages))
	}
	for i, created := range queries.createdMessages {
		archived := messages[i]
		if created.ExternalMessageID != archived.ExternalMessageID ||
			created.SourceReplyToMessageID != archived.SourceReplyToMessageID ||
			created.Role != archived.Role ||
			string(created.Content) != string(archived.Content) ||
			string(created.Usage) != string(archived.Usage) ||
			created.SessionMode != archived.SessionMode ||
			created.RuntimeType != archived.RuntimeType ||
			created.DisplayText != archived.DisplayText {
			t.Fatalf("created message %d changed archived fields: %#v", i, created)
		}
		if created.SessionID != targetSessionID {
			t.Fatalf("created message %d session = %s, want %s", i, created.SessionID.String(), targetSessionID.String())
		}
		wantLinked := i == 1
		if created.EventID.Valid != wantLinked {
			t.Fatalf("created message %d event link valid = %v, want %v", i, created.EventID.Valid, wantLinked)
		}
		if wantLinked && created.EventID != targetEventID {
			t.Fatalf("created message %d event = %s, want %s", i, created.EventID.String(), targetEventID.String())
		}
		var metadata map[string]json.RawMessage
		if err := json.Unmarshal(created.Metadata, &metadata); err != nil {
			t.Fatalf("decode created message %d metadata: %v", i, err)
		}
		if _, exists := metadata["_migration_0115_history_event_dedup"]; exists {
			t.Fatalf("created message %d retained 0115 migration marker: %s", i, created.Metadata)
		}
		var label string
		if err := json.Unmarshal(metadata["label"], &label); err != nil || label != archived.DisplayText.String {
			t.Fatalf("created message %d metadata label = %q, want %q", i, label, archived.DisplayText.String)
		}
	}
}

func legacyEventLinkedMessage(id, botID, sessionID, eventID pgtype.UUID, label string, createdAt time.Time) dbsqlc.ListAllMessagesForBackupRow {
	return dbsqlc.ListAllMessagesForBackupRow{
		ID:                     id,
		BotID:                  botID,
		SessionID:              sessionID,
		ExternalMessageID:      pgtype.Text{String: "external-" + label, Valid: true},
		SourceReplyToMessageID: pgtype.Text{String: "reply-" + label, Valid: true},
		Role:                   "assistant",
		Content:                []byte(`{"text":"` + label + `"}`),
		Metadata:               []byte(`{"label":"` + label + `"}`),
		Usage:                  []byte(`{"input_tokens":1}`),
		SessionMode:            "chat",
		RuntimeType:            "model",
		EventID:                eventID,
		DisplayText:            pgtype.Text{String: label, Valid: true},
		CreatedAt:              pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}

type recordingLegacyEventRestoreQueries struct {
	dbstore.Queries
	targetSessionID pgtype.UUID
	targetEventID   pgtype.UUID
	createdMessages []dbsqlc.CreateMessageParams
	linkedEvents    map[string]struct{}
}

func (q *recordingLegacyEventRestoreQueries) CreateSession(_ context.Context, arg dbsqlc.CreateSessionParams) (dbsqlc.BotSession, error) {
	return dbsqlc.BotSession{ID: q.targetSessionID, Metadata: arg.Metadata}, nil
}

func (*recordingLegacyEventRestoreQueries) NextSessionEventCursor(context.Context) (int64, error) {
	return 100, nil
}

func (q *recordingLegacyEventRestoreQueries) CreateSessionEvent(context.Context, dbsqlc.CreateSessionEventParams) (pgtype.UUID, error) {
	return q.targetEventID, nil
}

func (q *recordingLegacyEventRestoreQueries) CreateMessage(_ context.Context, arg dbsqlc.CreateMessageParams) (dbsqlc.CreateMessageRow, error) {
	if arg.EventID.Valid {
		key := arg.EventID.String()
		if _, exists := q.linkedEvents[key]; exists {
			return dbsqlc.CreateMessageRow{}, errors.New("duplicate restored event link")
		}
		q.linkedEvents[key] = struct{}{}
	}
	q.createdMessages = append(q.createdMessages, arg)
	return dbsqlc.CreateMessageRow{
		ID:        newBotBackupTestUUID(),
		SessionID: arg.SessionID,
		Role:      arg.Role,
	}, nil
}

func (*recordingLegacyEventRestoreQueries) CreateHistoryTurn(context.Context, dbsqlc.CreateHistoryTurnParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{ID: newBotBackupTestUUID()}, nil
}

func (*recordingLegacyEventRestoreQueries) BindHistoryTurnAssistantByRequest(context.Context, dbsqlc.BindHistoryTurnAssistantByRequestParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*recordingLegacyEventRestoreQueries) LinkMessageToHistoryTurn(_ context.Context, arg dbsqlc.LinkMessageToHistoryTurnParams) (pgtype.UUID, error) {
	return arg.MessageID, nil
}
