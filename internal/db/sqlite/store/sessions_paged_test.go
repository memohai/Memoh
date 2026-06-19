package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// TestSQLiteListSessionsByBotPagedRespectsCursorAndTypes pins down the bound-
// argument order for the paged session queries. The previous SQL mixed
// `sqlc.slice(types)` with explicit numbered placeholders, which silently
// misrouted the cursor and limit binds — this test exercises the SQLite shim
// against a real in-memory database so that class of bug fails loudly.
func TestSQLiteListSessionsByBotPagedRespectsCursorAndTypes(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Hand-rolled minimal schema rather than the full SQLite migration
	// baseline — see the comment in TestSQLiteListSessionsByBotPagedTiesOnUpdatedAt
	// for the rationale; schema drift on the queried columns surfaces via
	// the integration test path.
	execAll(t, conn, `
CREATE TABLE bot_channel_routes (
  id TEXT PRIMARY KEY,
  metadata TEXT,
  conversation_type TEXT
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  route_id TEXT REFERENCES bot_channel_routes(id),
  channel_type TEXT,
  type TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  parent_session_id TEXT,
  created_by_user_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);
`)

	botID := "11111111-1111-1111-1111-111111111111"

	// Five sessions across types, separated by one minute each so the cursor's
	// (updated_at, id) ordering is observable.
	type seed struct {
		id      string
		typ     string
		minutes int
	}
	seeds := []seed{
		{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01", "chat", 1},
		{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa02", "discuss", 2},
		{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa03", "heartbeat", 3},
		{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa04", "chat", 4},
		{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa05", "discuss", 5},
	}
	for _, s := range seeds {
		ts := time.Date(2026, 6, 19, 12, s.minutes, 0, 0, time.UTC)
		stored := ts.Format("2006-01-02 15:04:05")
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO bot_sessions (id, bot_id, type, updated_at, created_at) VALUES (?, ?, ?, ?, ?)`,
			s.id, botID, s.typ, stored, stored,
		); err != nil {
			t.Fatalf("insert seed %s: %v", s.id, err)
		}
	}

	st, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	queries := NewQueries(st)

	pgBotID := mustUUID(t, botID)

	// First page (limit 2, no cursor): two newest user-facing sessions; the
	// heartbeat row must be skipped.
	page1, err := queries.ListSessionsByBotPaged(ctx, pgsqlc.ListSessionsByBotPagedParams{
		BotID:      pgBotID,
		Types:      []string{"chat", "discuss"},
		UseCursor:  false,
		LimitCount: 2,
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("first page len = %d, want 2 (ids %v)", len(page1), pageIDs(page1))
	}
	if got := page1[0].ID.String(); !strings.HasSuffix(got, "05") {
		t.Fatalf("first page row 0 id = %s, want trailing 05", got)
	}
	if got := page1[1].ID.String(); !strings.HasSuffix(got, "04") {
		t.Fatalf("first page row 1 id = %s, want trailing 04", got)
	}
	for _, row := range page1 {
		if row.Type == "heartbeat" {
			t.Fatalf("heartbeat row leaked into user-facing page: %#v", row)
		}
	}

	// Cursor at the last row of page1 — the next page must skip rows 05 and 04
	// and surface only the older user-facing rows in correct order.
	cursor := page1[1]
	page2, err := queries.ListSessionsByBotPaged(ctx, pgsqlc.ListSessionsByBotPagedParams{
		BotID:           pgBotID,
		Types:           []string{"chat", "discuss"},
		UseCursor:       true,
		CursorUpdatedAt: pgtype.Timestamptz{Time: cursor.UpdatedAt.Time, Valid: true},
		CursorID:        cursor.ID,
		LimitCount:      10,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("second page len = %d, want 2 (ids %v)", len(page2), pageIDs(page2))
	}
	if got := page2[0].ID.String(); !strings.HasSuffix(got, "02") {
		t.Fatalf("second page row 0 id = %s, want trailing 02", got)
	}
	if got := page2[1].ID.String(); !strings.HasSuffix(got, "01") {
		t.Fatalf("second page row 1 id = %s, want trailing 01", got)
	}

	// Filter that explicitly includes heartbeat must surface every row.
	pageAll, err := queries.ListSessionsByBotPaged(ctx, pgsqlc.ListSessionsByBotPagedParams{
		BotID:      pgBotID,
		Types:      []string{"chat", "discuss", "heartbeat"},
		UseCursor:  false,
		LimitCount: 10,
	})
	if err != nil {
		t.Fatalf("page-all: %v", err)
	}
	if len(pageAll) != 5 {
		t.Fatalf("page-all len = %d, want 5 (ids %v)", len(pageAll), pageIDs(pageAll))
	}
}

func pageIDs(rows []pgsqlc.ListSessionsByBotPagedRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID.String()
	}
	return out
}

// TestSQLiteListSessionsByBotPagedTiesOnUpdatedAt exercises the
// (updated_at = ? AND id < ?) tiebreak branch of the cursor SQL. The cursor
// timestamp is bound at second precision against TEXT storage, so two rows
// inserted in the same second collide on the timestamp and only the id
// comparison can separate them. The other paged test seeds rows a minute
// apart and so never touches this branch.
func TestSQLiteListSessionsByBotPagedTiesOnUpdatedAt(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Hand-rolled minimal schema — matches the pattern in the rest of this
	// package's tests; running the full SQLite migration baseline (895+
	// lines, plus 20+ incremental files) for one unit test is not worth the
	// infra weight, and schema drift on the columns this query touches will
	// surface in the integration test path.
	execAll(t, conn, `
CREATE TABLE bot_channel_routes (
  id TEXT PRIMARY KEY,
  metadata TEXT,
  conversation_type TEXT
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  route_id TEXT REFERENCES bot_channel_routes(id),
  channel_type TEXT,
  type TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  parent_session_id TEXT,
  created_by_user_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);
`)

	botID := "22222222-2222-2222-2222-222222222222"
	sharedTS := "2026-06-19 00:00:00"
	ids := []string{
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01",
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02",
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03",
	}
	for _, id := range ids {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO bot_sessions (id, bot_id, type, updated_at, created_at) VALUES (?, ?, 'chat', ?, ?)`,
			id, botID, sharedTS, sharedTS,
		); err != nil {
			t.Fatalf("insert seed %s: %v", id, err)
		}
	}

	st, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	queries := NewQueries(st)
	pgBotID := mustUUID(t, botID)

	page1, err := queries.ListSessionsByBotPaged(ctx, pgsqlc.ListSessionsByBotPagedParams{
		BotID:      pgBotID,
		Types:      []string{"chat"},
		UseCursor:  false,
		LimitCount: 2,
	})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2 (ids %v)", len(page1), pageIDs(page1))
	}
	// id DESC within the same timestamp: 03 then 02.
	if got := page1[0].ID.String(); got != ids[2] {
		t.Fatalf("page1[0] = %s, want %s", got, ids[2])
	}
	if got := page1[1].ID.String(); got != ids[1] {
		t.Fatalf("page1[1] = %s, want %s", got, ids[1])
	}

	// Build the cursor exactly the way the handler does — from the last row.
	last := page1[1]
	page2, err := queries.ListSessionsByBotPaged(ctx, pgsqlc.ListSessionsByBotPagedParams{
		BotID:           pgBotID,
		Types:           []string{"chat"},
		UseCursor:       true,
		CursorUpdatedAt: pgtype.Timestamptz{Time: last.UpdatedAt.Time, Valid: true},
		CursorID:        last.ID,
		LimitCount:      10,
	})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1 (ids %v)", len(page2), pageIDs(page2))
	}
	if got := page2[0].ID.String(); got != ids[0] {
		t.Fatalf("page2[0] = %s, want %s (no duplicate, no skip)", got, ids[0])
	}
}
