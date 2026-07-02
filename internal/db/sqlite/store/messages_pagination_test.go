package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteListMessagesBySessionPaginatesBoundedTurnPath(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1000)
	sessionID := pageUUID(1001)
	const turnCount = 20
	base := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	seedLinearTurnChain(t, ctx, conn, botID, sessionID, turnCount, 2, base)
	headTurnID := pageTurnID(turnCount)
	insertPaginationSession(t, ctx, conn, botID, sessionID, headTurnID)
	q := newPaginationQueries(t, conn)

	latest, err := q.ListMessagesLatestBySession(ctx, pgsqlc.ListMessagesLatestBySessionParams{
		SessionID: mustUUID(t, sessionID),
		MaxCount:  5,
	})
	if err != nil {
		t.Fatalf("latest messages: %v", err)
	}
	assertMessageIDs(t, latestIDs(latest), []string{
		pageMessageID(20, 2),
		pageMessageID(20, 1),
		pageMessageID(19, 2),
		pageMessageID(19, 1),
		pageMessageID(18, 2),
	})

	beforeCursor := pageMessageID(19, 2)
	before, err := q.ListMessagesBeforeBySession(ctx, pgsqlc.ListMessagesBeforeBySessionParams{
		SessionID: mustUUID(t, sessionID),
		BeforeID:  mustUUID(t, beforeCursor),
		CreatedAt: pgtype.Timestamptz{Time: base.Add(19*10*time.Second + time.Second), Valid: true},
		MaxCount:  5,
	})
	if err != nil {
		t.Fatalf("before messages: %v", err)
	}
	assertMessageIDs(t, beforeIDs(before), []string{
		pageMessageID(19, 1),
		pageMessageID(18, 2),
		pageMessageID(18, 1),
		pageMessageID(17, 2),
		pageMessageID(17, 1),
	})
}

func TestSQLiteListMessagesLatestBySessionSingleTurnExceedsPageSize(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1100)
	sessionID := pageUUID(1101)
	turnID := pageTurnID(1)
	base := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)

	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, parent_turn_id) VALUES (?, ?, NULL)`, turnID, botID); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	for seq := 1; seq <= 8; seq++ {
		messageID := pageMessageID(1, seq)
		createdAt := base.Add(time.Duration(seq) * time.Second).Format("2006-01-02 15:04:05")
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, turn_id, turn_message_seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			messageID,
			botID,
			sessionID,
			turnID,
			seq,
			"assistant",
			fmt.Sprintf(`{"role":"assistant","content":%q}`, messageID),
			createdAt,
		); err != nil {
			t.Fatalf("insert message %s: %v", messageID, err)
		}
	}
	insertPaginationSession(t, ctx, conn, botID, sessionID, turnID)
	q := newPaginationQueries(t, conn)

	latest, err := q.ListMessagesLatestBySession(ctx, pgsqlc.ListMessagesLatestBySessionParams{
		SessionID: mustUUID(t, sessionID),
		MaxCount:  5,
	})
	if err != nil {
		t.Fatalf("latest messages: %v", err)
	}
	assertMessageIDs(t, latestIDs(latest), []string{
		pageMessageID(1, 8),
		pageMessageID(1, 7),
		pageMessageID(1, 6),
		pageMessageID(1, 5),
		pageMessageID(1, 4),
	})
}

func TestSQLiteListMessagesBeforeBySessionDeepCreatedAtCursor(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1200)
	sessionID := pageUUID(1201)
	const turnCount = 20
	base := time.Date(2026, 7, 2, 11, 0, 0, 0, time.UTC)
	seedLinearTurnChain(t, ctx, conn, botID, sessionID, turnCount, 2, base)
	insertPaginationSession(t, ctx, conn, botID, sessionID, pageTurnID(turnCount))
	q := newPaginationQueries(t, conn)

	cursorTime := base.Add(time.Duration(5*10+1) * time.Second)
	before, err := q.ListMessagesBeforeBySession(ctx, pgsqlc.ListMessagesBeforeBySessionParams{
		SessionID: mustUUID(t, sessionID),
		CreatedAt: pgtype.Timestamptz{Time: cursorTime, Valid: true},
		MaxCount:  5,
	})
	if err != nil {
		t.Fatalf("before messages: %v", err)
	}
	assertMessageIDs(t, beforeIDs(before), []string{
		pageMessageID(5, 1),
		pageMessageID(4, 2),
		pageMessageID(4, 1),
		pageMessageID(3, 2),
		pageMessageID(3, 1),
	})
}

func TestSQLiteListMessagesBeforeBySessionDeepBeforeIDCursor(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1300)
	sessionID := pageUUID(1301)
	const turnCount = 30
	base := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	seedLinearTurnChain(t, ctx, conn, botID, sessionID, turnCount, 2, base)
	insertPaginationSession(t, ctx, conn, botID, sessionID, pageTurnID(turnCount))
	q := newPaginationQueries(t, conn)

	beforeCursor := pageMessageID(5, 2)
	before, err := q.ListMessagesBeforeBySession(ctx, pgsqlc.ListMessagesBeforeBySessionParams{
		SessionID: mustUUID(t, sessionID),
		BeforeID:  mustUUID(t, beforeCursor),
		CreatedAt: pgtype.Timestamptz{Time: base.Add(time.Duration(5*10+1) * time.Second), Valid: true},
		MaxCount:  5,
	})
	if err != nil {
		t.Fatalf("before messages: %v", err)
	}
	assertMessageIDs(t, beforeIDs(before), []string{
		pageMessageID(5, 1),
		pageMessageID(4, 2),
		pageMessageID(4, 1),
		pageMessageID(3, 2),
		pageMessageID(3, 1),
	})
}

func TestSQLiteListMessagesBeforeBySessionCursorOffActivePath(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1400)
	sessionID := pageUUID(1401)
	base := time.Date(2026, 7, 2, 13, 0, 0, 0, time.UTC)

	turnRoot := pageTurnID(1)
	turnSibling := pageTurnID(2)
	turnOnPath := pageTurnID(3)
	headTurn := pageTurnID(4)
	offPathMessage := pageMessageID(2, 1)

	for _, item := range []struct {
		id, parent string
	}{
		{turnRoot, ""},
		{turnSibling, turnRoot},
		{turnOnPath, turnRoot},
		{headTurn, turnOnPath},
	} {
		var parent any
		if item.parent != "" {
			parent = item.parent
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, parent_turn_id) VALUES (?, ?, ?)`, item.id, botID, parent); err != nil {
			t.Fatalf("insert turn %s: %v", item.id, err)
		}
	}
	for _, item := range []struct {
		id, turnID string
		seq        int
	}{
		{pageMessageID(3, 1), turnOnPath, 1},
		{pageMessageID(3, 2), turnOnPath, 2},
		{pageMessageID(4, 1), headTurn, 1},
		{pageMessageID(4, 2), headTurn, 2},
		{offPathMessage, turnSibling, 1},
	} {
		createdAt := base.Add(time.Duration(item.seq) * time.Second).Format("2006-01-02 15:04:05")
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, turn_id, turn_message_seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			item.turnID,
			item.seq,
			"assistant",
			fmt.Sprintf(`{"role":"assistant","content":%q}`, item.id),
			createdAt,
		); err != nil {
			t.Fatalf("insert message %s: %v", item.id, err)
		}
	}
	insertPaginationSession(t, ctx, conn, botID, sessionID, headTurn)
	q := newPaginationQueries(t, conn)

	before, err := q.ListMessagesBeforeBySession(ctx, pgsqlc.ListMessagesBeforeBySessionParams{
		SessionID: mustUUID(t, sessionID),
		BeforeID:  mustUUID(t, offPathMessage),
		CreatedAt: pgtype.Timestamptz{Time: base.Add(2 * time.Second), Valid: true},
		MaxCount:  5,
	})
	if err != nil {
		t.Fatalf("before messages: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("off-path cursor should return no rows, got %d", len(before))
	}
}

func openPaginationDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(ctx, `
CREATE TABLE channel_identities (
  id TEXT PRIMARY KEY,
  display_name TEXT,
  avatar_url TEXT
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT,
  default_head_turn_id TEXT,
  deleted_at TEXT,
  channel_type TEXT
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  bot_id TEXT,
  parent_turn_id TEXT
);
CREATE TABLE bot_session_turn_heads (
  session_id TEXT NOT NULL,
  head_turn_id TEXT NOT NULL,
  bot_id TEXT NOT NULL,
  PRIMARY KEY (session_id, head_turn_id)
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  turn_id TEXT,
  turn_message_seq INTEGER,
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
  event_id TEXT,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	return conn
}

func newPaginationQueries(t *testing.T, conn *sql.DB) *Queries {
	t.Helper()
	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return NewQueries(store)
}

func insertPaginationSession(t *testing.T, ctx context.Context, conn *sql.DB, botID, sessionID, headTurnID string) {
	t.Helper()
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, default_head_turn_id, channel_type) VALUES (?, ?, ?, ?)`, sessionID, botID, headTurnID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id) VALUES (?, ?, ?)`, sessionID, headTurnID, botID); err != nil {
		t.Fatalf("insert session turn head: %v", err)
	}
}

func seedLinearTurnChain(t *testing.T, ctx context.Context, conn *sql.DB, botID, sessionID string, turnCount, messagesPerTurn int, base time.Time) {
	t.Helper()
	for turn := 1; turn <= turnCount; turn++ {
		turnID := pageTurnID(turn)
		var parent any
		if turn > 1 {
			parent = pageTurnID(turn - 1)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, parent_turn_id) VALUES (?, ?, ?)`, turnID, botID, parent); err != nil {
			t.Fatalf("insert turn %d: %v", turn, err)
		}
		for seq := 1; seq <= messagesPerTurn; seq++ {
			role := "user"
			if seq%2 == 0 {
				role = "assistant"
			}
			messageID := pageMessageID(turn, seq)
			createdAt := base.Add(time.Duration(turn*10+(seq-1)) * time.Second).Format("2006-01-02 15:04:05")
			if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, turn_id, turn_message_seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				messageID,
				botID,
				sessionID,
				turnID,
				seq,
				role,
				fmt.Sprintf(`{"role":%q,"content":%q}`, role, messageID),
				createdAt,
			); err != nil {
				t.Fatalf("insert message %s: %v", messageID, err)
			}
		}
	}
}

func pageUUID(n int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", n)
}

func pageTurnID(turn int) string {
	return pageUUID(2000 + turn)
}

func pageMessageID(turn, seq int) string {
	return pageUUID(3000 + turn*10 + seq)
}

func latestIDs(rows []pgsqlc.ListMessagesLatestBySessionRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID.String())
	}
	return out
}

func beforeIDs(rows []pgsqlc.ListMessagesBeforeBySessionRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID.String())
	}
	return out
}

func assertMessageIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("message ids = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message ids = %#v, want %#v", got, want)
		}
	}
}
