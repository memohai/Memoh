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
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, session_id, position) VALUES (?, ?, ?, 1)`, turnID, botID, sessionID); err != nil {
		t.Fatalf("insert existing turn: %v", err)
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
	existingTurnID := "00000000-0000-0000-0000-000000004103"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 7)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, session_id, position) VALUES (?, ?, ?, 1)`, existingTurnID, botID, sessionID); err != nil {
		t.Fatalf("insert existing turn: %v", err)
	}

	turn, err := q.CreateHistoryTurn(ctx, pgsqlc.CreateHistoryTurnParams{
		BotID:     pgUUIDFromString(t, botID),
		SessionID: pgUUIDFromString(t, sessionID),
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
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 7)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, session_id, position) VALUES (?, ?, ?, 1)`, turnID, botID, sessionID); err != nil {
		t.Fatalf("insert existing turn: %v", err)
	}

	_, err = q.CreateHistoryTurnWithID(ctx, pgsqlc.CreateHistoryTurnWithIDParams{
		TurnID:    pgUUIDFromString(t, turnID),
		BotID:     pgUUIDFromString(t, botID),
		SessionID: pgUUIDFromString(t, sessionID),
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
INSERT INTO bot_history_turns (id, bot_id, session_id, position)
VALUES (?, ?, ?, 1), (?, ?, ?, 2)`, oldTurnID, botID, sessionID, newerTurnID, botID, sessionID); err != nil {
		t.Fatalf("insert turns: %v", err)
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
INSERT INTO bot_history_turns (id, bot_id, session_id, position)
VALUES (?, ?, ?, 1)`, oldTurnID, botID, sessionID); err != nil {
		t.Fatalf("insert old turn: %v", err)
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
INSERT INTO bot_history_turns (id, bot_id, session_id, position)
VALUES (?, ?, ?, 1)`, otherTurnID, botID, otherSessionID); err != nil {
		t.Fatalf("insert other turn: %v", err)
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
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
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
`
