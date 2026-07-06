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
  next_turn_position INTEGER NOT NULL DEFAULT 1,
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
  turn_superseded_by_turn_id TEXT,
  turn_superseded_at TEXT,
  turn_superseded_reason TEXT,
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
CREATE VIEW bot_history_turns AS
SELECT
  m.turn_id AS id,
  (
    SELECT first_message.bot_id
    FROM bot_history_messages first_message
    WHERE first_message.turn_id = m.turn_id
      AND first_message.session_id = m.session_id
    ORDER BY first_message.turn_message_seq ASC, first_message.created_at ASC, first_message.id ASC
    LIMIT 1
  ) AS bot_id,
  m.session_id,
  m.turn_position AS position,
  COALESCE((
    SELECT request_message.id
    FROM bot_history_messages request_message
    WHERE request_message.turn_id = m.turn_id
      AND request_message.session_id = m.session_id
      AND request_message.role = 'user'
      AND request_message.turn_message_seq = 1
    ORDER BY request_message.created_at ASC, request_message.id ASC
    LIMIT 1
  ), '') AS request_message_id,
  COALESCE((
    SELECT assistant_message.id
    FROM bot_history_messages assistant_message
    WHERE assistant_message.turn_id = m.turn_id
      AND assistant_message.session_id = m.session_id
      AND assistant_message.role = 'assistant'
      AND assistant_message.turn_message_seq = 2
    ORDER BY assistant_message.created_at ASC, assistant_message.id ASC
    LIMIT 1
  ), '') AS assistant_message_id,
  (
    SELECT superseded_message.turn_superseded_by_turn_id
    FROM bot_history_messages superseded_message
    WHERE superseded_message.turn_id = m.turn_id
      AND superseded_message.session_id = m.session_id
      AND superseded_message.turn_superseded_by_turn_id IS NOT NULL
    ORDER BY superseded_message.turn_superseded_at DESC, superseded_message.created_at DESC, superseded_message.id DESC
    LIMIT 1
  ) AS superseded_by_turn_id,
  MAX(m.turn_superseded_at) AS superseded_at,
  (
    SELECT superseded_message.turn_superseded_reason
    FROM bot_history_messages superseded_message
    WHERE superseded_message.turn_id = m.turn_id
      AND superseded_message.session_id = m.session_id
      AND superseded_message.turn_superseded_reason IS NOT NULL
    ORDER BY superseded_message.turn_superseded_at DESC, superseded_message.created_at DESC, superseded_message.id DESC
    LIMIT 1
  ) AS superseded_reason,
  MIN(m.created_at) AS created_at,
  COALESCE(MAX(m.turn_superseded_at), MAX(m.created_at)) AS updated_at
FROM bot_history_messages m
WHERE m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.session_id IS NOT NULL
GROUP BY m.turn_id, m.session_id, m.turn_position;
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
