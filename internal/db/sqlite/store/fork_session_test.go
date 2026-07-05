package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteForkSessionPreservesCopiedMessageCreatedAt(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteForkSessionTestSchema)

	const (
		botID       = "00000000-0000-0000-0000-000000004001"
		sessionID   = "00000000-0000-0000-0000-000000004002"
		userID      = "00000000-0000-0000-0000-000000004003"
		assistantID = "00000000-0000-0000-0000-000000004004"
		turnID      = "00000000-0000-0000-0000-000000004005"
		userTime    = "2026-06-01 10:00:00.123"
		replyTime   = "2026-06-01 10:00:00.456"
	)
	if _, err := conn.ExecContext(ctx, `INSERT INTO bots (id) VALUES (?)`, botID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_sessions (id, bot_id, channel_type, type, title, metadata)
VALUES (?, ?, 'local', 'chat', 'source', '{}')`, sessionID, botID); err != nil {
		t.Fatalf("insert source session: %v", err)
	}
	for _, item := range []struct {
		id        string
		role      string
		content   string
		createdAt string
	}{
		{userID, "user", `{"role":"user","content":"hello"}`, userTime},
		{assistantID, "assistant", `{"role":"assistant","content":"hi"}`, replyTime},
	} {
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, item.id, botID, sessionID, item.role, item.content, item.createdAt); err != nil {
			t.Fatalf("insert message %s: %v", item.id, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, bot_id, session_id, position, request_message_id, assistant_message_id)
VALUES (?, ?, ?, 1, ?, ?)`, turnID, botID, sessionID, userID, assistantID); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
UPDATE bot_history_messages
SET turn_id = ?,
    turn_position = 1,
    turn_visible = 1,
    turn_message_seq = CASE id WHEN ? THEN 1 WHEN ? THEN 2 END
WHERE id IN (?, ?)`, turnID, userID, assistantID, userID, assistantID); err != nil {
		t.Fatalf("link source messages: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)
	forked, err := q.ForkSessionFromAssistantMessage(ctx, pgsqlc.ForkSessionFromAssistantMessageParams{
		SessionID: mustUUID(t, sessionID),
		BotID:     mustUUID(t, botID),
		MessageID: mustUUID(t, assistantID),
		Title:     "source fork",
		Metadata:  []byte(`{"forked_from":{"session_id":"` + sessionID + `","message_id":"` + assistantID + `"}}`),
	})
	if err != nil {
		t.Fatalf("fork session: %v", err)
	}

	var meta struct {
		ForkedFrom struct {
			ForkMessageID string `json:"fork_message_id"`
		} `json:"forked_from"`
	}
	if err := json.Unmarshal(forked.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal fork metadata: %v", err)
	}
	if meta.ForkedFrom.ForkMessageID == "" {
		t.Fatal("fork metadata missing fork_message_id")
	}

	var copiedCreatedAt string
	err = conn.QueryRowContext(ctx, `
SELECT created_at
FROM bot_history_messages
WHERE session_id = ? AND id = ?
`, forked.ID.String(), meta.ForkedFrom.ForkMessageID).Scan(&copiedCreatedAt)
	if err != nil {
		t.Fatalf("load copied assistant: %v", err)
	}
	if copiedCreatedAt != replyTime {
		t.Fatalf("copied assistant created_at = %q, want %q", copiedCreatedAt, replyTime)
	}
	assertSQLiteForkAnchorVisible(t, ctx, conn, forked.ID.String(), meta.ForkedFrom.ForkMessageID)
}

func TestSQLiteForkSessionCopiesCompleteAssistantTurn(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteForkSessionTestSchema)

	const (
		botID       = "00000000-0000-0000-0000-000000004101"
		sessionID   = "00000000-0000-0000-0000-000000004102"
		userID      = "00000000-0000-0000-0000-000000004103"
		assistantID = "00000000-0000-0000-0000-000000004104"
		tailID      = "00000000-0000-0000-0000-000000004105"
		turnID      = "00000000-0000-0000-0000-000000004106"
		turnPos     = int64(7)
	)
	if _, err := conn.ExecContext(ctx, `INSERT INTO bots (id) VALUES (?)`, botID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_sessions (id, bot_id, channel_type, type, title, metadata)
VALUES (?, ?, 'local', 'chat', 'source', '{}')`, sessionID, botID); err != nil {
		t.Fatalf("insert source session: %v", err)
	}
	for _, item := range []struct {
		id        string
		role      string
		content   string
		createdAt string
	}{
		{userID, "user", `{"role":"user","content":"hello"}`, "2026-06-01 10:00:00.100"},
		{assistantID, "assistant", `{"role":"assistant","content":"tool call"}`, "2026-06-01 10:00:00.200"},
		{tailID, "assistant", `{"role":"assistant","content":"final answer"}`, "2026-06-01 10:00:00.300"},
	} {
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, item.id, botID, sessionID, item.role, item.content, item.createdAt); err != nil {
			t.Fatalf("insert message %s: %v", item.id, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, bot_id, session_id, position, request_message_id, assistant_message_id)
VALUES (?, ?, ?, ?, ?, ?)`, turnID, botID, sessionID, turnPos, userID, assistantID); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
UPDATE bot_history_messages
SET turn_id = ?,
    turn_position = ?,
    turn_visible = 1,
    turn_message_seq = CASE id WHEN ? THEN 1 WHEN ? THEN 2 WHEN ? THEN 3 END
WHERE id IN (?, ?, ?)`, turnID, turnPos, userID, assistantID, tailID, userID, assistantID, tailID); err != nil {
		t.Fatalf("link source messages: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)
	forked, err := q.ForkSessionFromAssistantMessage(ctx, pgsqlc.ForkSessionFromAssistantMessageParams{
		SessionID: mustUUID(t, sessionID),
		BotID:     mustUUID(t, botID),
		MessageID: mustUUID(t, assistantID),
		Title:     "source fork",
		Metadata:  []byte(`{"forked_from":{"session_id":"` + sessionID + `","message_id":"` + assistantID + `"}}`),
	})
	if err != nil {
		t.Fatalf("fork session: %v", err)
	}

	var meta struct {
		ForkedFrom struct {
			ForkMessageID string `json:"fork_message_id"`
		} `json:"forked_from"`
	}
	if err := json.Unmarshal(forked.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal fork metadata: %v", err)
	}

	var copiedContent string
	err = conn.QueryRowContext(ctx, `
SELECT content
FROM bot_history_messages
WHERE session_id = ? AND id = ?
`, forked.ID.String(), meta.ForkedFrom.ForkMessageID).Scan(&copiedContent)
	if err != nil {
		t.Fatalf("load copied fork anchor: %v", err)
	}
	if copiedContent != `{"role":"assistant","content":"tool call"}` {
		t.Fatalf("fork anchor copied content = %q, want first assistant message", copiedContent)
	}
	assertSQLiteForkAnchorVisible(t, ctx, conn, forked.ID.String(), meta.ForkedFrom.ForkMessageID)
	assertSQLiteForkCopiedContentVisible(t, ctx, conn, forked.ID.String(), `{"role":"assistant","content":"final answer"}`)
	assertSQLiteForkMessagePositionsMatchTurns(t, ctx, conn, forked.ID.String())
}

func assertSQLiteForkAnchorVisible(t *testing.T, ctx context.Context, conn *sql.DB, sessionID, messageID string) {
	t.Helper()

	var visibleCount int
	err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM bot_visible_history_messages
WHERE session_id = ? AND id = ?
`, sessionID, messageID).Scan(&visibleCount)
	if err != nil {
		t.Fatalf("count visible fork anchor: %v", err)
	}
	if visibleCount != 1 {
		t.Fatalf("visible fork anchor count = %d, want 1", visibleCount)
	}
}

func assertSQLiteForkCopiedContentVisible(t *testing.T, ctx context.Context, conn *sql.DB, sessionID, content string) {
	t.Helper()

	var visibleCount int
	err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM bot_visible_history_messages
WHERE session_id = ? AND content = ?
`, sessionID, content).Scan(&visibleCount)
	if err != nil {
		t.Fatalf("count visible copied content: %v", err)
	}
	if visibleCount != 1 {
		t.Fatalf("visible copied content count = %d, want 1", visibleCount)
	}
}

func assertSQLiteForkMessagePositionsMatchTurns(t *testing.T, ctx context.Context, conn *sql.DB, sessionID string) {
	t.Helper()

	var mismatchCount int
	err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = ?
  AND m.turn_position IS NOT t.position
`, sessionID).Scan(&mismatchCount)
	if err != nil {
		t.Fatalf("count fork turn_position mismatches: %v", err)
	}
	if mismatchCount != 0 {
		t.Fatalf("fork copied message turn_position mismatches linked turn count = %d, want 0", mismatchCount)
	}
}

const sqliteForkSessionTestSchema = `
CREATE TABLE bots (
  id TEXT PRIMARY KEY
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY DEFAULT (
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-' ||
    '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
  ),
  bot_id TEXT NOT NULL,
  route_id TEXT,
  channel_type TEXT,
  type TEXT NOT NULL DEFAULT 'chat',
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  runtime_metadata TEXT NOT NULL DEFAULT '{}',
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  parent_session_id TEXT,
  created_by_user_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  turn_id TEXT,
  turn_position INTEGER,
  turn_message_seq INTEGER,
  turn_visible INTEGER NOT NULL DEFAULT 0,
  sender_channel_identity_id TEXT,
  sender_account_user_id TEXT,
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  model_id TEXT,
  event_id TEXT,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_message_assets (
  id TEXT PRIMARY KEY DEFAULT (
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-' ||
    '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
  ),
  message_id TEXT NOT NULL,
  role TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY DEFAULT (
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-' ||
    '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
  ),
  bot_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  request_message_id TEXT,
  assistant_message_id TEXT,
  superseded_by_turn_id TEXT,
  superseded_at TEXT,
  superseded_reason TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (session_id, position)
);
CREATE VIEW bot_visible_history_messages AS
SELECT
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
  m.session_mode,
  m.runtime_type,
  m.model_id,
  m.event_id,
  m.display_text,
  m.created_at
FROM bot_history_messages m
WHERE m.turn_visible = 1;
`

var _ = sql.ErrNoRows
