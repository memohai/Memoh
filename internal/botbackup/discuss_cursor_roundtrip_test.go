package botbackup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
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
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-30", `{"event_cursor":30,"text":"thirty"}`, 30),
			backupRoundTripEvent(sourceBotID, sourceSessionB, "b-15", `{"event_cursor":15,"text":"other session"}`, 15),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-invalid", `{"event_cursor":"invalid","text":"legacy invalid"}`, 40),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-10", `{"event_cursor":10,"text":"ten"}`, 10),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-missing", `{"text":"legacy missing"}`, 50),
			backupRoundTripEvent(sourceBotID, sourceSessionA, "a-20", `{"event_cursor":20,"text":"twenty"}`, 20),
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

func TestRestoreHistoryKeepsMixedLegacyAndCursorEventReplayOrder(t *testing.T) {
	ctx := context.Background()
	sourceBotID := newBotBackupTestUUID()
	targetBotID := newBotBackupTestUUID()
	actorUserID := newBotBackupTestUUID()
	sourceSessionID := newBotBackupTestUUID()
	targetSessionID := newBotBackupTestUUID()

	queries := &recordingDiscussCursorRoundTripQueries{
		createdSessionIDsByTitle: map[string]pgtype.UUID{"mixed": targetSessionID},
		nextEventCursors:         []int64{9001, 9002},
	}
	state := &importState{
		entries: map[string]backupZipEntry{
			"history/sessions.json": jsonBackupEntry(t, []dbsqlc.ListSessionsByBotRow{
				backupRoundTripSession(sourceBotID, sourceSessionID, "mixed"),
			}),
			"history/messages.json": jsonBackupEntry(t, []dbsqlc.ListAllMessagesForBackupRow{}),
			"history/session_events.json": jsonBackupEntry(t, []dbsqlc.BotSessionEvent{
				{
					ID:                newBotBackupTestUUID(),
					BotID:             sourceBotID,
					SessionID:         sourceSessionID,
					EventKind:         string(pipelinepkg.EventEdit),
					EventData:         []byte(`{"session_id":"source","message_id":"message","received_at_ms":200,"event_cursor":200,"content":[{"type":"text","text":"after"}],"attachments":[]}`),
					ExternalMessageID: pgtype.Text{String: "edit", Valid: true},
					ReceivedAtMs:      200,
				},
				{
					ID:                newBotBackupTestUUID(),
					BotID:             sourceBotID,
					SessionID:         sourceSessionID,
					EventKind:         string(pipelinepkg.EventMessage),
					EventData:         []byte(`{"session_id":"source","message_id":"message","received_at_ms":100,"content":[{"type":"text","text":"before"}],"attachments":[],"conversation":{"channel":"local","conversation_type":"direct"}}`),
					ExternalMessageID: pgtype.Text{String: "message", Valid: true},
					ReceivedAtMs:      100,
				},
			}),
			"history/discuss_cursors.json": jsonBackupEntry(t, []dbsqlc.BotSessionDiscussCursor{
				{
					SessionID:           sourceSessionID,
					ScopeKey:            "source:telegram",
					Source:              "telegram",
					ConsumedEventCursor: 200,
				},
			}),
		},
		counts: map[Section]int{},
	}

	svc := &Service{queries: queries}
	if err := svc.restoreHistory(ctx, actorUserID.String(), targetBotID.String(), state, false, false); err != nil {
		t.Fatalf("restoreHistory() error = %v", err)
	}

	restoredEvents := make([]pipelinepkg.CanonicalEvent, 0, len(queries.createdEvents))
	for _, created := range queries.createdEvents {
		switch created.EventKind {
		case string(pipelinepkg.EventMessage):
			var event pipelinepkg.MessageEvent
			if err := json.Unmarshal(created.EventData, &event); err != nil {
				t.Fatalf("decode restored message event: %v", err)
			}
			restoredEvents = append(restoredEvents, event)
		case string(pipelinepkg.EventEdit):
			var event pipelinepkg.EditEvent
			if err := json.Unmarshal(created.EventData, &event); err != nil {
				t.Fatalf("decode restored edit event: %v", err)
			}
			restoredEvents = append(restoredEvents, event)
		default:
			t.Fatalf("unexpected restored event kind %q", created.EventKind)
		}
	}

	rendered := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{}).ReplaySession(targetSessionID.String(), restoredEvents)
	if len(rendered) != 1 || len(rendered[0].Content) != 1 || !strings.Contains(rendered[0].Content[0].Text, "after") {
		t.Fatalf("restored replay = %#v, want message followed by edit", rendered)
	}
	if len(queries.upsertedCursors) != 1 || queries.upsertedCursors[0].ConsumedEventCursor != 9002 {
		t.Fatalf("restored discuss cursors = %#v, want consumed event cursor 9002", queries.upsertedCursors)
	}
}

func TestRestoreHistoryPreservesCompletedEventWithoutClaims(t *testing.T) {
	ctx := context.Background()
	sourceBotID := newBotBackupTestUUID()
	targetBotID := newBotBackupTestUUID()
	actorUserID := newBotBackupTestUUID()
	sourceSessionID := newBotBackupTestUUID()
	targetSessionID := newBotBackupTestUUID()
	targetCompletedEventID := newBotBackupTestUUID()
	targetIncompleteEventID := newBotBackupTestUUID()
	completedAt := time.Date(2026, time.July, 17, 19, 30, 45, 123456000, time.UTC)
	claimUntil := completedAt.Add(time.Hour)

	completed := backupRoundTripEvent(
		sourceBotID,
		sourceSessionID,
		"completed",
		`{"event_cursor":10,"text":"completed"}`,
		10,
	)
	completed.DeliveryClaimToken = newBotBackupTestUUID()
	completed.DeliveryClaimedUntil = pgtype.Timestamptz{Time: claimUntil, Valid: true}
	completed.DeliveryCompletedAt = pgtype.Timestamptz{Time: completedAt, Valid: true}
	incomplete := backupRoundTripEvent(
		sourceBotID,
		sourceSessionID,
		"incomplete",
		`{"event_cursor":20,"text":"incomplete"}`,
		20,
	)
	incomplete.DeliveryClaimToken = newBotBackupTestUUID()
	incomplete.DeliveryClaimedUntil = pgtype.Timestamptz{Time: claimUntil, Valid: true}

	queries := &recordingDiscussCursorRoundTripQueries{
		createdSessionIDsByTitle:    map[string]pgtype.UUID{"completion": targetSessionID},
		nextEventCursors:            []int64{9001, 9002},
		nextCreatedEventIDs:         []pgtype.UUID{targetCompletedEventID, targetIncompleteEventID},
		restoredEventCompletionRows: 1,
	}
	state := &importState{
		entries: map[string]backupZipEntry{
			"history/sessions.json": jsonBackupEntry(t, []dbsqlc.ListSessionsByBotRow{
				backupRoundTripSession(sourceBotID, sourceSessionID, "completion"),
			}),
			"history/messages.json":       jsonBackupEntry(t, []dbsqlc.ListAllMessagesForBackupRow{}),
			"history/session_events.json": jsonBackupEntry(t, []dbsqlc.BotSessionEvent{incomplete, completed}),
		},
		counts: map[Section]int{},
	}

	if err := (&Service{queries: queries}).restoreHistory(
		ctx,
		actorUserID.String(),
		targetBotID.String(),
		state,
		false,
		false,
	); err != nil {
		t.Fatalf("restoreHistory() error = %v", err)
	}
	if len(queries.createdEvents) != 2 {
		t.Fatalf("created events = %d, want 2", len(queries.createdEvents))
	}
	if len(queries.restoredEventCompletions) != 1 {
		t.Fatalf("restored event completions = %d, want 1", len(queries.restoredEventCompletions))
	}
	restored := queries.restoredEventCompletions[0]
	if restored.EventID != targetCompletedEventID || !restored.DeliveryCompletedAt.Valid || !restored.DeliveryCompletedAt.Time.Equal(completedAt) {
		t.Fatalf("restored event completion = %#v, want event %s at %s", restored, targetCompletedEventID.String(), completedAt)
	}
}

func TestRestoreHistoryRequiresCompletedEventUpdate(t *testing.T) {
	sourceBotID := newBotBackupTestUUID()
	targetBotID := newBotBackupTestUUID()
	actorUserID := newBotBackupTestUUID()
	sourceSessionID := newBotBackupTestUUID()
	targetSessionID := newBotBackupTestUUID()
	completed := backupRoundTripEvent(
		sourceBotID,
		sourceSessionID,
		"completed",
		`{"event_cursor":10}`,
		10,
	)
	completed.DeliveryCompletedAt = pgtype.Timestamptz{
		Time:  time.Date(2026, time.July, 17, 19, 30, 45, 0, time.UTC),
		Valid: true,
	}
	queries := &recordingDiscussCursorRoundTripQueries{
		createdSessionIDsByTitle: map[string]pgtype.UUID{"completion": targetSessionID},
		nextEventCursors:         []int64{9001},
		nextCreatedEventIDs:      []pgtype.UUID{newBotBackupTestUUID()},
	}
	state := &importState{
		entries: map[string]backupZipEntry{
			"history/sessions.json": jsonBackupEntry(t, []dbsqlc.ListSessionsByBotRow{
				backupRoundTripSession(sourceBotID, sourceSessionID, "completion"),
			}),
			"history/messages.json":       jsonBackupEntry(t, []dbsqlc.ListAllMessagesForBackupRow{}),
			"history/session_events.json": jsonBackupEntry(t, []dbsqlc.BotSessionEvent{completed}),
		},
		counts: map[Section]int{},
	}

	err := (&Service{queries: queries}).restoreHistory(
		context.Background(),
		actorUserID.String(),
		targetBotID.String(),
		state,
		false,
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "updated 0 rows, want 1") {
		t.Fatalf("restoreHistory() error = %v, want exact completion row error", err)
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

func backupRoundTripEvent(botID, sessionID pgtype.UUID, externalID, data string, receivedAtMs int64) dbsqlc.BotSessionEvent {
	return dbsqlc.BotSessionEvent{
		ID:                newBotBackupTestUUID(),
		BotID:             botID,
		SessionID:         sessionID,
		EventKind:         "message",
		EventData:         []byte(data),
		ExternalMessageID: pgtype.Text{String: externalID, Valid: true},
		ReceivedAtMs:      receivedAtMs,
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
	exportedSessions            []dbsqlc.ListSessionsByBotRow
	exportedCursors             []dbsqlc.BotSessionDiscussCursor
	exportedEvents              []dbsqlc.BotSessionEvent
	createdSessionIDsByTitle    map[string]pgtype.UUID
	nextEventCursors            []int64
	allocatedEventCursors       []int64
	nextCreatedEventIDs         []pgtype.UUID
	createdEvents               []dbsqlc.CreateSessionEventParams
	restoredEventCompletions    []dbsqlc.RestoreSessionEventDeliveryCompletionParams
	restoredEventCompletionRows int64
	upsertedCursors             []dbsqlc.UpsertSessionDiscussCursorParams
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
	if len(q.nextCreatedEventIDs) > 0 {
		id := q.nextCreatedEventIDs[0]
		q.nextCreatedEventIDs = q.nextCreatedEventIDs[1:]
		return id, nil
	}
	return newBotBackupTestUUID(), nil
}

func (q *recordingDiscussCursorRoundTripQueries) RestoreSessionEventDeliveryCompletion(
	_ context.Context,
	arg dbsqlc.RestoreSessionEventDeliveryCompletionParams,
) (int64, error) {
	q.restoredEventCompletions = append(q.restoredEventCompletions, arg)
	return q.restoredEventCompletionRows, nil
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
