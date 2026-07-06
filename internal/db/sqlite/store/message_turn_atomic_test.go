package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteCreateMessageWithHistoryTurnRollsBackOnTurnFailure(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004001"
	sessionID := "00000000-0000-0000-0000-000000004002"
	turnID := "00000000-0000-0000-0000-000000004003"
	messageID := "00000000-0000-0000-0000-000000004004"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 7)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_position, turn_message_seq, turn_visible)
VALUES (?, ?, ?, 'user', '{}', ?, 1, 1, 1)`, "00000000-0000-0000-0000-000000004005", botID, sessionID, turnID); err != nil {
		t.Fatalf("insert existing turn message: %v", err)
	}

	_, err = q.CreateMessageWithHistoryTurn(ctx, pgsqlc.CreateMessageWithHistoryTurnParams{
		MessageID:      pgUUIDFromString(t, messageID),
		BotID:          pgUUIDFromString(t, botID),
		SessionID:      pgUUIDFromString(t, sessionID),
		Role:           "user",
		Content:        []byte(`{"type":"text","content":"rollback"}`),
		Metadata:       []byte(`{}`),
		Usage:          []byte(`{}`),
		SessionMode:    "chat",
		RuntimeType:    "model",
		TurnID:         pgUUIDFromString(t, turnID),
		TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
	})
	if err == nil {
		t.Fatal("create message with duplicate turn id succeeded, want error")
	}

	var messageCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_history_messages WHERE id = ?`, messageID).Scan(&messageCount); err != nil {
		t.Fatalf("count message: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("message count after rollback = %d, want 0", messageCount)
	}
	var nextPosition int64
	if err := conn.QueryRowContext(ctx, `SELECT next_turn_position FROM bot_sessions WHERE id = ?`, sessionID).Scan(&nextPosition); err != nil {
		t.Fatalf("select next position: %v", err)
	}
	if nextPosition != 7 {
		t.Fatalf("next_turn_position after rollback = %d, want 7", nextPosition)
	}
}

func TestSQLiteCreateMessageWithHistoryTurnUsesAllocatedPosition(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004301"
	sessionID := "00000000-0000-0000-0000-000000004302"
	messageID := "00000000-0000-0000-0000-000000004303"
	turnID := "00000000-0000-0000-0000-000000004304"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 7)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	_, err = q.CreateMessageWithHistoryTurn(ctx, pgsqlc.CreateMessageWithHistoryTurnParams{
		MessageID:      pgUUIDFromString(t, messageID),
		BotID:          pgUUIDFromString(t, botID),
		SessionID:      pgUUIDFromString(t, sessionID),
		Role:           "user",
		Content:        []byte(`{"type":"text","content":"hello"}`),
		Metadata:       []byte(`{}`),
		Usage:          []byte(`{}`),
		SessionMode:    "chat",
		RuntimeType:    "model",
		TurnID:         pgUUIDFromString(t, turnID),
		TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
	})
	if err != nil {
		t.Fatalf("create message with history turn: %v", err)
	}

	var messageTurnID string
	var messageTurnPosition int64
	var messageTurnSeq int64
	var messageVisible int64
	if err := conn.QueryRowContext(ctx, `
SELECT turn_id, turn_position, turn_message_seq, turn_visible
FROM bot_history_messages
WHERE id = ?`, messageID).Scan(&messageTurnID, &messageTurnPosition, &messageTurnSeq, &messageVisible); err != nil {
		t.Fatalf("select message turn fields: %v", err)
	}
	if messageTurnID != turnID || messageTurnPosition != 7 || messageTurnSeq != 1 || messageVisible != 1 {
		t.Fatalf("message turn fields = (%s, %d, %d, %d), want (%s, 7, 1, 1)", messageTurnID, messageTurnPosition, messageTurnSeq, messageVisible, turnID)
	}

	var turnPosition int64
	var requestMessageID string
	if err := conn.QueryRowContext(ctx, `
SELECT position, request_message_id
FROM bot_history_turns
WHERE id = ?`, turnID).Scan(&turnPosition, &requestMessageID); err != nil {
		t.Fatalf("select created turn: %v", err)
	}
	if turnPosition != 7 || requestMessageID != messageID {
		t.Fatalf("created turn = (%d, %s), want (7, %s)", turnPosition, requestMessageID, messageID)
	}

	var nextPosition int64
	if err := conn.QueryRowContext(ctx, `SELECT next_turn_position FROM bot_sessions WHERE id = ?`, sessionID).Scan(&nextPosition); err != nil {
		t.Fatalf("select next position: %v", err)
	}
	if nextPosition != 8 {
		t.Fatalf("next_turn_position after create = %d, want 8", nextPosition)
	}
}

func TestSQLiteCreateHistoryTurnUsesSessionAllocator(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004101"
	sessionID := "00000000-0000-0000-0000-000000004102"
	requestMessageID := "00000000-0000-0000-0000-000000004103"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 7)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content)
VALUES (?, ?, ?, 'user', '{}')`, requestMessageID, botID, sessionID); err != nil {
		t.Fatalf("insert request message: %v", err)
	}

	turn, err := q.CreateHistoryTurn(ctx, pgsqlc.CreateHistoryTurnParams{
		BotID:            pgUUIDFromString(t, botID),
		SessionID:        pgUUIDFromString(t, sessionID),
		RequestMessageID: pgUUIDFromString(t, requestMessageID),
	})
	if err != nil {
		t.Fatalf("create history turn: %v", err)
	}
	if turn.Position != 7 {
		t.Fatalf("created turn position = %d, want allocator position 7", turn.Position)
	}
	var nextPosition int64
	if err := conn.QueryRowContext(ctx, `SELECT next_turn_position FROM bot_sessions WHERE id = ?`, sessionID).Scan(&nextPosition); err != nil {
		t.Fatalf("select next position: %v", err)
	}
	if nextPosition != 8 {
		t.Fatalf("next_turn_position after create = %d, want 8", nextPosition)
	}
}

func TestSQLiteCreateHistoryTurnWithIDRollsBackAllocatorOnInsertFailure(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004201"
	sessionID := "00000000-0000-0000-0000-000000004202"
	turnID := "00000000-0000-0000-0000-000000004203"
	requestMessageID := "00000000-0000-0000-0000-000000004204"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 7)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_position, turn_message_seq, turn_visible)
VALUES (?, ?, ?, 'user', '{}', ?, 1, 1, 1)`, "00000000-0000-0000-0000-000000004205", botID, sessionID, turnID); err != nil {
		t.Fatalf("insert existing turn message: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content)
VALUES (?, ?, ?, 'user', '{}')`, requestMessageID, botID, sessionID); err != nil {
		t.Fatalf("insert request message: %v", err)
	}

	_, err = q.CreateHistoryTurnWithID(ctx, pgsqlc.CreateHistoryTurnWithIDParams{
		TurnID:           pgUUIDFromString(t, turnID),
		BotID:            pgUUIDFromString(t, botID),
		SessionID:        pgUUIDFromString(t, sessionID),
		RequestMessageID: pgUUIDFromString(t, requestMessageID),
	})
	if err == nil {
		t.Fatal("create duplicate history turn succeeded, want error")
	}
	var nextPosition int64
	if err := conn.QueryRowContext(ctx, `SELECT next_turn_position FROM bot_sessions WHERE id = ?`, sessionID).Scan(&nextPosition); err != nil {
		t.Fatalf("select next position: %v", err)
	}
	if nextPosition != 7 {
		t.Fatalf("next_turn_position after rollback = %d, want 7", nextPosition)
	}
}

func TestSQLiteReplaceHistoryTurnRequiresLatestVisibleTurn(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004501"
	sessionID := "00000000-0000-0000-0000-000000004502"
	oldTurnID := "00000000-0000-0000-0000-000000004503"
	newerTurnID := "00000000-0000-0000-0000-000000004504"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 3)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_position, turn_message_seq, turn_visible)
VALUES
  (?, ?, ?, 'user', '{}', ?, 1, 1, 1),
  (?, ?, ?, 'user', '{}', ?, 2, 1, 1)`,
		"00000000-0000-0000-0000-000000004505", botID, sessionID, oldTurnID,
		"00000000-0000-0000-0000-000000004506", botID, sessionID, newerTurnID); err != nil {
		t.Fatalf("insert turn messages: %v", err)
	}

	_, err = q.ReplaceHistoryTurn(ctx, pgsqlc.ReplaceHistoryTurnParams{
		OldTurnID:        pgUUIDFromString(t, oldTurnID),
		SessionID:        pgUUIDFromString(t, sessionID),
		SupersededReason: pgtype.Text{String: "retry", Valid: true},
	})
	if err == nil {
		t.Fatal("replace stale turn succeeded, want error")
	}
	var supersededCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_history_turns WHERE superseded_at IS NOT NULL`).Scan(&supersededCount); err != nil {
		t.Fatalf("count superseded turns: %v", err)
	}
	if supersededCount != 0 {
		t.Fatalf("superseded turns = %d, want 0", supersededCount)
	}
	var nextPosition int64
	if err := conn.QueryRowContext(ctx, `SELECT next_turn_position FROM bot_sessions WHERE id = ?`, sessionID).Scan(&nextPosition); err != nil {
		t.Fatalf("select next position: %v", err)
	}
	if nextPosition != 3 {
		t.Fatalf("next_turn_position after stale replace = %d, want 3", nextPosition)
	}
}

func TestSQLiteReplaceHistoryTurnValidatesAnchorsBeforeAllocating(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004601"
	sessionID := "00000000-0000-0000-0000-000000004602"
	oldTurnID := "00000000-0000-0000-0000-000000004603"
	wrongRoleMessageID := "00000000-0000-0000-0000-000000004604"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 2)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_position, turn_message_seq, turn_visible)
VALUES (?, ?, ?, 'user', '{}', ?, 1, 1, 1)`, "00000000-0000-0000-0000-000000004605", botID, sessionID, oldTurnID); err != nil {
		t.Fatalf("insert old turn message: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content)
VALUES (?, ?, ?, 'assistant', '{}')`, wrongRoleMessageID, botID, sessionID); err != nil {
		t.Fatalf("insert wrong-role anchor: %v", err)
	}

	_, err = q.ReplaceHistoryTurn(ctx, pgsqlc.ReplaceHistoryTurnParams{
		OldTurnID:        pgUUIDFromString(t, oldTurnID),
		SessionID:        pgUUIDFromString(t, sessionID),
		RequestMessageID: pgUUIDFromString(t, wrongRoleMessageID),
		SupersededReason: pgtype.Text{String: "retry", Valid: true},
	})
	if err == nil {
		t.Fatal("replace with wrong-role request anchor succeeded, want error")
	}
	var nextPosition int64
	if err := conn.QueryRowContext(ctx, `SELECT next_turn_position FROM bot_sessions WHERE id = ?`, sessionID).Scan(&nextPosition); err != nil {
		t.Fatalf("select next position: %v", err)
	}
	if nextPosition != 2 {
		t.Fatalf("next_turn_position after invalid anchor = %d, want 2", nextPosition)
	}
	var turnCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_history_turns`).Scan(&turnCount); err != nil {
		t.Fatalf("count turns: %v", err)
	}
	if turnCount != 1 {
		t.Fatalf("turn count after invalid anchor = %d, want 1", turnCount)
	}
}

func TestSQLiteLinkMessageToHistoryTurnRequiresExistingTurn(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004701"
	sessionID := "00000000-0000-0000-0000-000000004702"
	messageID := "00000000-0000-0000-0000-000000004703"
	missingTurnID := "00000000-0000-0000-0000-000000004704"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 1)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content)
VALUES (?, ?, ?, 'user', '{}')`, messageID, botID, sessionID); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	_, err = q.LinkMessageToHistoryTurn(ctx, pgsqlc.LinkMessageToHistoryTurnParams{
		MessageID:      pgUUIDFromString(t, messageID),
		TurnID:         pgUUIDFromString(t, missingTurnID),
		TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
	})
	if err == nil {
		t.Fatal("link with missing turn succeeded, want error")
	}
	var linkedTurn sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT turn_id FROM bot_history_messages WHERE id = ?`, messageID).Scan(&linkedTurn); err != nil {
		t.Fatalf("select linked turn: %v", err)
	}
	if linkedTurn.Valid {
		t.Fatalf("message linked to missing turn %q", linkedTurn.String)
	}

	otherSessionID := "00000000-0000-0000-0000-000000004705"
	otherTurnID := "00000000-0000-0000-0000-000000004706"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 2)`, otherSessionID, botID); err != nil {
		t.Fatalf("insert other session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_position, turn_message_seq, turn_visible)
VALUES (?, ?, ?, 'user', '{}', ?, 1, 1, 1)`, "00000000-0000-0000-0000-000000004707", botID, otherSessionID, otherTurnID); err != nil {
		t.Fatalf("insert other turn message: %v", err)
	}

	_, err = q.LinkMessageToHistoryTurn(ctx, pgsqlc.LinkMessageToHistoryTurnParams{
		MessageID:      pgUUIDFromString(t, messageID),
		TurnID:         pgUUIDFromString(t, otherTurnID),
		TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
	})
	if err == nil {
		t.Fatal("cross-session link succeeded, want error")
	}
	linkedTurn = sql.NullString{}
	if err := conn.QueryRowContext(ctx, `SELECT turn_id FROM bot_history_messages WHERE id = ?`, messageID).Scan(&linkedTurn); err != nil {
		t.Fatalf("select linked turn after cross-session link: %v", err)
	}
	if linkedTurn.Valid {
		t.Fatalf("message linked to cross-session turn %q", linkedTurn.String)
	}
}

func pgUUIDFromString(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var out pgtype.UUID
	if err := out.Scan(value); err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return out
}

const sqliteCreateMessageWithHistoryTurnAtomicSchema = `
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  next_turn_position INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
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
  turn_id TEXT,
  turn_position INTEGER,
  turn_message_seq INTEGER,
  turn_visible INTEGER NOT NULL DEFAULT 0,
  turn_superseded_by_turn_id TEXT,
  turn_superseded_at TEXT,
  turn_superseded_reason TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;
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
`
