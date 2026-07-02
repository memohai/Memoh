package store

import (
	"context"
	"database/sql"
	"errors"
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

// seedVariantFixture builds the retry fixture shared by the view-by-turn
// query tests: two root turns where turnB is a retry sibling of turnA
// (inherited request group), plus turnC extending turnA. Active heads are
// turnC (session default, most recently updated) and turnB.
//
//	turnA (root) ── turnC   [head, default]
//	turnB (root, group=turnA)   [head]
func seedVariantFixture(t *testing.T, ctx context.Context, conn *sql.DB, botID, sessionID string) (turnA, turnB, turnC string) {
	t.Helper()
	base := time.Date(2026, 7, 2, 14, 0, 0, 0, time.UTC)
	turnA = pageTurnID(1)
	turnB = pageTurnID(2)
	turnC = pageTurnID(3)
	for i, item := range []struct {
		turnID, parentID, requestGroupID string
		hasAssistant                     bool
	}{
		{turnA, "", "", true},
		// turnB simulates a retry sibling that inherited turnA's group and
		// has no final assistant message yet.
		{turnB, "", turnA, false},
		{turnC, turnA, "", true},
	} {
		parent := sql.NullString{String: item.parentID, Valid: item.parentID != ""}
		group := sql.NullString{String: item.requestGroupID, Valid: item.requestGroupID != ""}
		assistant := sql.NullString{String: pageMessageID(i+1, 2), Valid: item.hasAssistant}
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, bot_id, parent_turn_id, request_message_id, final_assistant_message_id, request_group_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.turnID, botID, parent, pageMessageID(i+1, 1), assistant, group,
			base.Add(time.Duration(i*10)*time.Second).Format("2006-01-02 15:04:05"),
		); err != nil {
			t.Fatalf("insert turn %s: %v", item.turnID, err)
		}
	}
	insertPaginationSession(t, ctx, conn, botID, sessionID, turnC)
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id, updated_at)
VALUES (?, ?, ?, ?)`,
		sessionID, turnB, botID, base.Add(-time.Hour).Format("2006-01-02 15:04:05"),
	); err != nil {
		t.Fatalf("insert second head: %v", err)
	}
	return turnA, turnB, turnC
}

// TestSQLiteListSessionTurnSiblingsRootPage pins the page-level variant
// metadata query: a page containing a root turn aggregates every reachable
// root as its sibling set, request_group_id falls back to the turn's own id
// when NULL and passes through the stored group for retry siblings.
func TestSQLiteListSessionTurnSiblingsRootPage(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1500)
	sessionID := pageUUID(1501)
	turnA, turnB, _ := seedVariantFixture(t, ctx, conn, botID, sessionID)
	q := newPaginationQueries(t, conn)

	rows, err := q.ListSessionTurnSiblings(ctx, pgsqlc.ListSessionTurnSiblingsParams{
		SessionID: mustUUID(t, sessionID),
		TurnIds:   []pgtype.UUID{mustUUID(t, turnA)},
	})
	if err != nil {
		t.Fatalf("list turn siblings: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("sibling rows = %d, want 2", len(rows))
	}
	if rows[0].TurnID.String() != turnA || rows[1].TurnID.String() != turnB {
		t.Fatalf("sibling order = [%s %s], want [%s %s]", rows[0].TurnID, rows[1].TurnID, turnA, turnB)
	}
	if !rows[0].HasUser || !rows[0].HasAssistant {
		t.Fatalf("first row flags = user:%v assistant:%v, want both true", rows[0].HasUser, rows[0].HasAssistant)
	}
	if rows[1].HasAssistant {
		t.Fatal("retry sibling without final assistant message reported has_assistant=true")
	}
	if rows[0].RequestGroupID.String() != turnA {
		t.Fatalf("first row request group = %s, want self group %s", rows[0].RequestGroupID, turnA)
	}
	if rows[1].RequestGroupID.String() != turnA {
		t.Fatalf("second row request group = %s, want inherited group %s", rows[1].RequestGroupID, turnA)
	}
}

// TestSQLiteListSessionTurnSiblingsChildPage verifies a non-root page turn
// only aggregates children of the same parent — the sibling root fork stays
// out of the result.
func TestSQLiteListSessionTurnSiblingsChildPage(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1510)
	sessionID := pageUUID(1511)
	_, _, turnC := seedVariantFixture(t, ctx, conn, botID, sessionID)
	q := newPaginationQueries(t, conn)

	rows, err := q.ListSessionTurnSiblings(ctx, pgsqlc.ListSessionTurnSiblingsParams{
		SessionID: mustUUID(t, sessionID),
		TurnIds:   []pgtype.UUID{mustUUID(t, turnC)},
	})
	if err != nil {
		t.Fatalf("list turn siblings: %v", err)
	}
	if len(rows) != 1 || rows[0].TurnID.String() != turnC {
		t.Fatalf("sibling rows = %#v, want only %s", rows, turnC)
	}
}

// TestSQLiteResolveSessionTurnHead pins head resolution for variant
// switching: a non-head turn resolves to the head whose path contains it, a
// head resolves to itself, and a turn outside every head path resolves to
// nothing.
func TestSQLiteResolveSessionTurnHead(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1520)
	sessionID := pageUUID(1521)
	turnA, turnB, turnC := seedVariantFixture(t, ctx, conn, botID, sessionID)
	q := newPaginationQueries(t, conn)

	resolved, err := q.ResolveSessionTurnHead(ctx, pgsqlc.ResolveSessionTurnHeadParams{
		SessionID:    mustUUID(t, sessionID),
		TargetTurnID: mustUUID(t, turnA),
	})
	if err != nil {
		t.Fatalf("resolve ancestor turn: %v", err)
	}
	if resolved.String() != turnC {
		t.Fatalf("resolved head for %s = %s, want %s", turnA, resolved, turnC)
	}

	resolved, err = q.ResolveSessionTurnHead(ctx, pgsqlc.ResolveSessionTurnHeadParams{
		SessionID:    mustUUID(t, sessionID),
		TargetTurnID: mustUUID(t, turnB),
	})
	if err != nil {
		t.Fatalf("resolve head turn: %v", err)
	}
	if resolved.String() != turnB {
		t.Fatalf("resolved head for %s = %s, want itself", turnB, resolved)
	}

	if _, err := q.ResolveSessionTurnHead(ctx, pgsqlc.ResolveSessionTurnHeadParams{
		SessionID:    mustUUID(t, sessionID),
		TargetTurnID: mustUUID(t, pageTurnID(99)),
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("resolve unknown turn err = %v, want sql.ErrNoRows", err)
	}
}

// TestSQLiteListSessionTurnPathIDs pins the SSE path query: the head's
// ancestor chain, self included, and nothing from sibling branches.
func TestSQLiteListSessionTurnPathIDs(t *testing.T) {
	ctx := context.Background()
	conn := openPaginationDB(t, ctx)
	botID := pageUUID(1530)
	sessionID := pageUUID(1531)
	turnA, turnB, turnC := seedVariantFixture(t, ctx, conn, botID, sessionID)
	q := newPaginationQueries(t, conn)

	ids, err := q.ListSessionTurnPathIDs(ctx, mustUUID(t, turnC))
	if err != nil {
		t.Fatalf("list path ids: %v", err)
	}
	got := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		got[id.String()] = struct{}{}
	}
	if len(got) != 2 {
		t.Fatalf("path ids = %v, want {%s %s}", ids, turnC, turnA)
	}
	for _, want := range []string{turnA, turnC} {
		if _, ok := got[want]; !ok {
			t.Fatalf("path ids = %v, missing %s", ids, want)
		}
	}
	if _, ok := got[turnB]; ok {
		t.Fatalf("path ids leaked sibling branch turn %s: %v", turnB, ids)
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
  parent_turn_id TEXT,
  request_message_id TEXT,
  final_assistant_message_id TEXT,
  origin_kind TEXT,
  origin_turn_id TEXT,
  request_group_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_message_assets (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL,
  role TEXT,
  ordinal INTEGER,
  content_hash TEXT,
  name TEXT,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_session_turn_heads (
  session_id TEXT NOT NULL,
  head_turn_id TEXT NOT NULL,
  bot_id TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
