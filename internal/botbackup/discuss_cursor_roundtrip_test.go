package botbackup

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestHistoryBackupRoundTripRemapsEventAndDiscussCursors(t *testing.T) {
	ctx := context.Background()
	sourceBotID := newBotBackupTestUUID()
	targetBotID := newBotBackupTestUUID()
	actorUserID := newBotBackupTestUUID()
	sourceSessionA := newBotBackupTestUUID()
	sourceSessionB := newBotBackupTestUUID()
	targetSessionA := newBotBackupTestUUID()
	targetSessionB := newBotBackupTestUUID()

	queries := &recordingDiscussCursorRoundTripQueries{
		exportedSessions: []dbsqlc.ListSessionsByBotRow{
			backupRoundTripSession(sourceBotID, sourceSessionA, "session-a"),
			backupRoundTripSession(sourceBotID, sourceSessionB, "session-b"),
		},
		exportedCursors: []dbsqlc.BotSessionDiscussCursor{
			{
				SessionID:           sourceSessionA,
				ScopeKey:            "source:telegram",
				Source:              "telegram",
				ConsumedCursor:      1234,
				ConsumedEventCursor: 18,
			},
			{
				SessionID:           sourceSessionA,
				ScopeKey:            "route:room",
				Source:              "telegram",
				ConsumedCursor:      5678,
				ConsumedEventCursor: 25,
			},
			{
				SessionID:           sourceSessionA,
				ScopeKey:            "source:no-match",
				Source:              "telegram",
				ConsumedCursor:      7890,
				ConsumedEventCursor: 5,
			},
			{
				SessionID:           sourceSessionA,
				ScopeKey:            "legacy:invalid-boundary",
				Source:              "telegram",
				ConsumedCursor:      9012,
				ConsumedEventCursor: -1,
			},
		},
		exportedEvents: []dbsqlc.BotSessionEvent{
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-30", `{"event_cursor":30,"text":"thirty"}`),
			backupRoundTripEvent(sourceBotID, sourceSessionB, "b-15", `{"event_cursor":15,"text":"other session"}`),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-invalid", `{"event_cursor":"invalid","text":"legacy invalid"}`),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-10", `{"event_cursor":10,"text":"ten"}`),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-missing", `{"text":"legacy missing"}`),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-20", `{"event_cursor":20,"text":"twenty"}`),
		},
		createdSessionIDsByTitle: map[string]pgtype.UUID{
			"session-a": targetSessionA,
			"session-b": targetSessionB,
		},
		nextEventCursors: []int64{9001, 9002, 9003, 9004, 9005, 9006},
	}
	svc := &Service{queries: queries}

	history, warnings := svc.collectHistory(ctx, sourceBotID.String(), false)
	if len(warnings) != 0 {
		t.Fatalf("collectHistory() warnings = %v", warnings)
	}
	state := &importState{
		entries: map[string]backupZipEntry{
			"history/sessions.json":        jsonBackupEntry(t, history.Sessions),
			"history/messages.json":        jsonBackupEntry(t, history.Messages),
			"history/discuss_cursors.json": jsonBackupEntry(t, history.DiscussCursors),
			"history/session_events.json":  jsonBackupEntry(t, history.SessionEvents),
		},
		counts: map[Section]int{},
	}
	if err := svc.restoreHistory(ctx, actorUserID.String(), targetBotID.String(), state, false, false); err != nil {
		t.Fatalf("restoreHistory() error = %v", err)
	}

	wantEventOrder := []string{"a-10", "b-15", "a-20", "a-30", "a-invalid", "a-missing"}
	if len(queries.createdEvents) != len(wantEventOrder) {
		t.Fatalf("created events = %d, want %d", len(queries.createdEvents), len(wantEventOrder))
	}
	for i, externalID := range wantEventOrder {
		created := queries.createdEvents[i]
		if !created.ExternalMessageID.Valid || created.ExternalMessageID.String != externalID {
			t.Fatalf("created event %d external id = %#v, want %q", i, created.ExternalMessageID, externalID)
		}
		var data struct {
			EventCursor int64  `json:"event_cursor"`
			Text        string `json:"text"`
		}
		if err := json.Unmarshal(created.EventData, &data); err != nil {
			t.Fatalf("decode created event %q: %v", externalID, err)
		}
		if data.EventCursor != queries.allocatedEventCursors[i] {
			t.Fatalf("created event %q cursor = %d, want %d", externalID, data.EventCursor, queries.allocatedEventCursors[i])
		}
		if data.Text == "" {
			t.Fatalf("created event %q lost archived payload", externalID)
		}
	}

	wantDiscussCursors := map[string]struct {
		legacy int64
		event  int64
	}{
		"source:telegram":         {legacy: 1234, event: 9001},
		"route:room":              {legacy: 5678, event: 9003},
		"source:no-match":         {legacy: 7890, event: 0},
		"legacy:invalid-boundary": {legacy: 9012, event: 0},
	}
	if len(queries.upsertedCursors) != len(wantDiscussCursors) {
		t.Fatalf("upserted discuss cursors = %d, want %d", len(queries.upsertedCursors), len(wantDiscussCursors))
	}
	for _, cursor := range queries.upsertedCursors {
		want, ok := wantDiscussCursors[cursor.ScopeKey]
		if !ok {
			t.Fatalf("unexpected restored discuss cursor %q", cursor.ScopeKey)
		}
		if cursor.SessionID != targetSessionA {
			t.Fatalf("restored cursor %q session = %s, want %s", cursor.ScopeKey, cursor.SessionID.String(), targetSessionA.String())
		}
		if cursor.ConsumedCursor != want.legacy || cursor.ConsumedEventCursor != want.event {
			t.Fatalf(
				"restored cursor %q = legacy:%d event:%d, want legacy:%d event:%d",
				cursor.ScopeKey,
				cursor.ConsumedCursor,
				cursor.ConsumedEventCursor,
				want.legacy,
				want.event,
			)
		}
	}
}

func backupRoundTripSession(botID, sessionID pgtype.UUID, title string) dbsqlc.ListSessionsByBotRow {
	return dbsqlc.ListSessionsByBotRow{
		ID:              sessionID,
		BotID:           botID,
		ChannelType:     pgtype.Text{String: "local", Valid: true},
		Type:            "conversation",
		SessionMode:     "chat",
		RuntimeType:     "model",
		RuntimeMetadata: []byte(`{}`),
		Title:           title,
		Metadata:        []byte(`{}`),
	}
}

func backupRoundTripEvent(botID, sessionID pgtype.UUID, externalID, data string) dbsqlc.BotSessionEvent {
	return dbsqlc.BotSessionEvent{
		ID:                newBotBackupTestUUID(),
		BotID:             botID,
		SessionID:         sessionID,
		EventKind:         "message",
		EventData:         []byte(data),
		ExternalMessageID: pgtype.Text{String: externalID, Valid: true},
	}
}

func jsonBackupEntry(t *testing.T, value any) backupZipEntry {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal backup entry: %v", err)
	}
	return backupZipEntry{data: raw}
}

type recordingDiscussCursorRoundTripQueries struct {
	dbstore.Queries
	exportedSessions         []dbsqlc.ListSessionsByBotRow
	exportedCursors          []dbsqlc.BotSessionDiscussCursor
	exportedEvents           []dbsqlc.BotSessionEvent
	createdSessionIDsByTitle map[string]pgtype.UUID
	nextEventCursors         []int64
	allocatedEventCursors    []int64
	createdEvents            []dbsqlc.CreateSessionEventParams
	upsertedCursors          []dbsqlc.UpsertSessionDiscussCursorParams
}

func (q *recordingDiscussCursorRoundTripQueries) ListSessionsByBot(context.Context, pgtype.UUID) ([]dbsqlc.ListSessionsByBotRow, error) {
	return q.exportedSessions, nil
}

func (q *recordingDiscussCursorRoundTripQueries) ListSessionDiscussCursorsByBot(context.Context, pgtype.UUID) ([]dbsqlc.BotSessionDiscussCursor, error) {
	return q.exportedCursors, nil
}

func (q *recordingDiscussCursorRoundTripQueries) ListSessionEventsByBot(context.Context, pgtype.UUID) ([]dbsqlc.BotSessionEvent, error) {
	return q.exportedEvents, nil
}

func (*recordingDiscussCursorRoundTripQueries) ListAllMessagesForBackup(context.Context, pgtype.UUID) ([]dbsqlc.ListAllMessagesForBackupRow, error) {
	return nil, nil
}

func (q *recordingDiscussCursorRoundTripQueries) CreateSession(_ context.Context, arg dbsqlc.CreateSessionParams) (dbsqlc.BotSession, error) {
	return dbsqlc.BotSession{ID: q.createdSessionIDsByTitle[arg.Title], Metadata: arg.Metadata}, nil
}

func (q *recordingDiscussCursorRoundTripQueries) NextSessionEventCursor(context.Context) (int64, error) {
	cursor := q.nextEventCursors[0]
	q.nextEventCursors = q.nextEventCursors[1:]
	q.allocatedEventCursors = append(q.allocatedEventCursors, cursor)
	return cursor, nil
}

func (q *recordingDiscussCursorRoundTripQueries) CreateSessionEvent(_ context.Context, arg dbsqlc.CreateSessionEventParams) (pgtype.UUID, error) {
	q.createdEvents = append(q.createdEvents, arg)
	return newBotBackupTestUUID(), nil
}

func (q *recordingDiscussCursorRoundTripQueries) UpsertSessionDiscussCursor(_ context.Context, arg dbsqlc.UpsertSessionDiscussCursorParams) (dbsqlc.BotSessionDiscussCursor, error) {
	q.upsertedCursors = append(q.upsertedCursors, arg)
	return dbsqlc.BotSessionDiscussCursor{
		SessionID:           arg.SessionID,
		ScopeKey:            arg.ScopeKey,
		Source:              arg.Source,
		ConsumedCursor:      arg.ConsumedCursor,
		ConsumedEventCursor: arg.ConsumedEventCursor,
	}, nil
}
