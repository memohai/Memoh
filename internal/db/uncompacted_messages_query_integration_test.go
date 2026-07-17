package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// TestListUncompactedMessagesReclaimEligibility exercises the real SQL
// eligibility predicate against PostgreSQL: usable summaries and fresh pending
// leases stay excluded, while stale or failed claims are reclaimable.
func TestListUncompactedMessagesReclaimEligibility(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "uncompacted_query_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema+", public"); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	if _, err := tx.Exec(ctx, baseline); err != nil {
		t.Fatalf("apply 0001 baseline: %v", err)
	}
	bindTeamQueryFixture(t, ctx, tx)
	if _, err := tx.Exec(ctx, `
ALTER TABLE users ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE bots ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE bot_sessions ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE bot_channel_routes ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE channel_identities ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE bot_history_message_compacts ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE bot_history_messages ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE bot_session_events ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();

DROP VIEW bot_visible_history_messages;
CREATE VIEW bot_visible_history_messages AS
SELECT
  m.team_id,
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id,
  m.source_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.compact_id,
  m.session_mode,
  m.runtime_type,
  m.model_id,
  m.event_id,
  m.display_text,
  m.created_at
FROM bot_history_messages m
WHERE m.turn_visible = true
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL;
`); err != nil {
		t.Fatalf("add team query fixture schema: %v", err)
	}

	userID := uuid.NewString()
	botID := uuid.NewString()
	sessionID := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO users (id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bots (id, owner_user_id, type, name) VALUES ($1, $2, 'personal', 'reclaim-test')`, botID, userID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_sessions (id, bot_id) VALUES ($1, $2)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	validCursorEventID := uuid.NewString()
	zeroCursorEventID := uuid.NewString()
	malformedCursorEventID := uuid.NewString()
	overflowCursorEventID := uuid.NewString()
	missingCursorEventID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_events
  (id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms)
VALUES
  ($1, $6, $7, 'message', '{"event_cursor":42}', 'cursor-valid', 42),
  ($2, $6, $7, 'message', '{"event_cursor":0}', 'cursor-zero', 43),
  ($3, $6, $7, 'message', '{"event_cursor":"invalid"}', 'cursor-malformed', 44),
  ($4, $6, $7, 'message', '{"event_cursor":9007199254740992}', 'cursor-overflow', 45),
  ($5, $6, $7, 'message', '{}', 'cursor-missing', 9999)
`, validCursorEventID, zeroCursorEventID, malformedCursorEventID, overflowCursorEventID, missingCursorEventID, botID, sessionID); err != nil {
		t.Fatalf("insert source event: %v", err)
	}

	logs := map[string]string{
		"usable":       uuid.NewString(),
		"error":        uuid.NewString(),
		"pendingFresh": uuid.NewString(),
		"pendingStale": uuid.NewString(),
		"poison":       uuid.NewString(),
		"whitespace":   uuid.NewString(),
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary, started_at) VALUES
  ($1, $7, $8, 'ok', 'a usable summary', now()),
  ($2, $7, $8, 'error', '', now()),
  ($3, $7, $8, 'pending', '', now()),
  ($4, $7, $8, 'pending', '', now() - INTERVAL '16 minutes'),
  ($5, $7, $8, 'ok', '', now()),
  ($6, $7, $8, 'ok', E'  \n\t', now())
`, logs["usable"], logs["error"], logs["pendingFresh"], logs["pendingStale"], logs["poison"], logs["whitespace"], botID, sessionID); err != nil {
		t.Fatalf("insert compact logs: %v", err)
	}

	type fixture struct {
		name        string
		compactID   string
		eventID     string
		eventCursor int64
		metadata    string
		eligible    bool
	}
	fixtures := []fixture{
		{name: "valid event cursor", eventID: validCursorEventID, eventCursor: 42, eligible: true},
		{name: "zero event cursor", eventID: zeroCursorEventID, eligible: true},
		{name: "malformed event cursor", eventID: malformedCursorEventID, eligible: true},
		{name: "overflow event cursor", eventID: overflowCursorEventID, eligible: true},
		{name: "missing event cursor", eventID: missingCursorEventID, eligible: true},
		{name: "missing source event", eligible: true},
		{name: "covered by usable summary", compactID: logs["usable"], eligible: false},
		{name: "log failed", compactID: logs["error"], eligible: true},
		{name: "fresh pending lease", compactID: logs["pendingFresh"], eligible: false},
		{name: "stale pending lease", compactID: logs["pendingStale"], eligible: true},
		{name: "legacy ok with empty summary", compactID: logs["poison"], eligible: true},
		{name: "legacy ok with whitespace-only summary", compactID: logs["whitespace"], eligible: true},
		{name: "passive sync", metadata: `{"trigger_mode":"passive_sync"}`, eligible: false},
	}
	wantEligible := make(map[string]fixture)
	for i, f := range fixtures {
		id := uuid.NewString()
		metadata := f.metadata
		if metadata == "" {
			metadata = "{}"
		}
		var compactID any
		if f.compactID != "" {
			compactID = f.compactID
		}
		var eventID any
		if f.eventID != "" {
			eventID = f.eventID
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages
  (id, bot_id, session_id, role, content, metadata, compact_id, event_id, turn_visible, turn_id, turn_position, turn_message_seq, created_at)
VALUES
  ($1, $2, $3, 'user', '[{"type":"text","text":"m"}]', $4, $5, $6, true, $7, $8, 0, now() + make_interval(secs => $9))
`, id, botID, sessionID, metadata, compactID, eventID, uuid.NewString(), i, i); err != nil {
			t.Fatalf("insert message %q: %v", f.name, err)
		}
		if f.eligible {
			wantEligible[id] = f
		}
	}

	var sessionUUID pgtype.UUID
	if err := sessionUUID.Scan(sessionID); err != nil {
		t.Fatalf("scan session uuid: %v", err)
	}
	rows, err := sqlc.New(tx).ListUncompactedMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("ListUncompactedMessagesBySession: %v", err)
	}

	got := make(map[string]int64)
	for i, row := range rows {
		id := uuid.UUID(row.ID.Bytes).String()
		got[id] = row.EventCursor
		if i > 0 && row.CreatedAt.Time.Before(rows[i-1].CreatedAt.Time) {
			t.Fatalf("rows not ordered by created_at: %v after %v", row.CreatedAt.Time, rows[i-1].CreatedAt.Time)
		}
	}
	for id, expected := range wantEligible {
		cursor, ok := got[id]
		if !ok {
			t.Errorf("row %q missing from candidate set", expected.name)
		} else if cursor != expected.eventCursor {
			t.Errorf("row %q event cursor = %d, want %d", expected.name, cursor, expected.eventCursor)
		}
	}
	if len(got) != len(wantEligible) {
		t.Errorf("candidate set size = %d, want %d (an excluded row leaked in)", len(got), len(wantEligible))
	}
}

func TestListMessageEventCursorsByIDsUsesExactHistoryEventLinks(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "message_event_cursors_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema+", public"); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := tx.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")); err != nil {
		t.Fatalf("apply 0001 baseline: %v", err)
	}
	bindTeamQueryFixture(t, ctx, tx)
	if _, err := tx.Exec(ctx, `
ALTER TABLE bot_history_messages ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
ALTER TABLE bot_session_events ADD COLUMN team_id UUID NOT NULL DEFAULT public.memoh_current_team_id();
`); err != nil {
		t.Fatalf("add team cursor query fixture schema: %v", err)
	}

	userID := uuid.NewString()
	botID := uuid.NewString()
	sessionID := uuid.NewString()
	otherSessionID := uuid.NewString()
	compactID := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO users (id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bots (id, owner_user_id, type, name) VALUES ($1, $2, 'personal', 'cursor-query-test')`, botID, userID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_sessions (id, bot_id) VALUES ($1, $3), ($2, $3)`, sessionID, otherSessionID, botID); err != nil {
		t.Fatalf("insert sessions: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary)
VALUES ($1, $2, $3, 'ok', 'summary')
`, compactID, botID, sessionID); err != nil {
		t.Fatalf("insert compaction artifact: %v", err)
	}

	type eventFixture struct {
		id        string
		sessionID string
		data      string
		external  string
	}
	events := []eventFixture{
		{id: uuid.NewString(), sessionID: sessionID, data: `{"event_cursor":42}`, external: "event-valid"},
		{id: uuid.NewString(), sessionID: sessionID, data: `{"event_cursor":0}`, external: "event-zero"},
		{id: uuid.NewString(), sessionID: sessionID, data: `{"event_cursor":"invalid"}`, external: "event-malformed"},
		{id: uuid.NewString(), sessionID: sessionID, data: `{"event_cursor":9007199254740992}`, external: "event-overflow"},
		{id: uuid.NewString(), sessionID: sessionID, data: `{}`, external: "event-missing"},
		{id: uuid.NewString(), sessionID: otherSessionID, data: `{"event_cursor":77}`, external: "event-cross-session-link"},
		{id: uuid.NewString(), sessionID: otherSessionID, data: `{"event_cursor":99}`, external: "event-other-session"},
	}
	for index, event := range events {
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_events
  (id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms)
VALUES ($1, $2, $3, 'message', $4::jsonb, $5, $6)
`, event.id, botID, event.sessionID, event.data, event.external, index+1); err != nil {
			t.Fatalf("insert event %q: %v", event.external, err)
		}
	}

	type messageFixture struct {
		id        string
		sessionID string
		external  string
		reply     string
		eventID   any
		cursor    int64
	}
	messages := []messageFixture{
		{id: uuid.NewString(), sessionID: sessionID, external: "message-valid", reply: "reply-valid", eventID: events[0].id, cursor: 42},
		{id: uuid.NewString(), sessionID: sessionID, external: "message-zero", eventID: events[1].id},
		{id: uuid.NewString(), sessionID: sessionID, external: "message-malformed", eventID: events[2].id},
		{id: uuid.NewString(), sessionID: sessionID, external: "message-overflow", eventID: events[3].id},
		{id: uuid.NewString(), sessionID: sessionID, external: "message-missing", eventID: events[4].id},
		{id: uuid.NewString(), sessionID: sessionID, external: "message-no-event"},
		{id: uuid.NewString(), sessionID: sessionID, external: "message-cross-session-event", eventID: events[5].id},
		{id: uuid.NewString(), sessionID: otherSessionID, external: "message-other-session", eventID: events[6].id, cursor: 99},
	}
	requestedIDs := make([]pgtype.UUID, 0, len(messages))
	wantCursors := make(map[string]int64, len(messages)-1)
	for _, message := range messages {
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages
  (id, bot_id, session_id, source_message_id, source_reply_to_message_id, role, content, compact_id, event_id)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), 'user', '{}'::jsonb, $6, $7)
`, message.id, botID, message.sessionID, message.external, message.reply, compactID, message.eventID); err != nil {
			t.Fatalf("insert message %q: %v", message.external, err)
		}
		requestedIDs = append(requestedIDs, pgUUID(t, message.id))
		if message.sessionID == sessionID {
			wantCursors[message.id] = message.cursor
		}
	}

	rows, err := sqlc.New(tx).ListMessageEventCursorsByIDs(ctx, sqlc.ListMessageEventCursorsByIDsParams{
		MessageIds: requestedIDs,
		BotID:      pgUUID(t, botID),
		SessionID:  pgUUID(t, sessionID),
	})
	if err != nil {
		t.Fatalf("ListMessageEventCursorsByIDs: %v", err)
	}
	if len(rows) != len(wantCursors) {
		t.Fatalf("cursor rows = %d, want %d", len(rows), len(wantCursors))
	}
	for _, row := range rows {
		id := uuid.UUID(row.ID.Bytes).String()
		want, exists := wantCursors[id]
		if !exists {
			t.Fatalf("cursor query leaked row %s outside owner", id)
		}
		if row.EventCursor != want {
			t.Errorf("row %s event cursor = %d, want %d", id, row.EventCursor, want)
		}
		if row.CompactID != pgUUID(t, compactID) || row.BotID != pgUUID(t, botID) || row.SessionID != pgUUID(t, sessionID) {
			t.Errorf("row %s lost exact owner/artifact identity: %#v", id, row)
		}
		if id == messages[0].id && (!row.EventID.Valid || row.ExternalMessageID.String != "message-valid" || row.SourceReplyToMessageID.String != "reply-valid" || !row.CreatedAt.Valid) {
			t.Errorf("valid cursor row lost stable fields: %#v", row)
		}
	}
}
