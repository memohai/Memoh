package store

import (
	"context"
	"database/sql"
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

	createPagedSessionSchema(t, conn)

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

	parentID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01"
	otherParentID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02"
	childID := "cccccccc-cccc-cccc-cccc-cccccccccc01"
	otherChildID := "cccccccc-cccc-cccc-cccc-cccccccccc02"
	for _, s := range []struct {
		id       string
		parentID string
	}{
		{childID, parentID},
		{otherChildID, otherParentID},
	} {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO bot_sessions (id, bot_id, type, parent_session_id, updated_at, created_at) VALUES (?, ?, 'subagent', ?, ?, ?)`,
			s.id, botID, s.parentID, "2026-06-19 12:10:00", "2026-06-19 12:10:00",
		); err != nil {
			t.Fatalf("insert subagent seed %s: %v", s.id, err)
		}
	}
	parentPage, err := queries.ListSessionsByBotPaged(ctx, pgsqlc.ListSessionsByBotPagedParams{
		BotID:            pgBotID,
		Types:            []string{"subagent"},
		UseParentSession: true,
		ParentSessionID:  mustUUID(t, parentID),
		LimitCount:       10,
	})
	if err != nil {
		t.Fatalf("parent-filtered page: %v", err)
	}
	if len(parentPage) != 1 || parentPage[0].ID.String() != childID {
		t.Fatalf("parent-filtered ids = %v, want only %s", pageIDs(parentPage), childID)
	}
}

func pageIDs(rows []pgsqlc.ListSessionsByBotPagedRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID.String()
	}
	return out
}

// TestSQLiteListSessionsByBotPagedWalksSameSecondRows pins down the second-
// precision cursor invariant. Rows sharing the same SQLite timestamp must be
// separated by the `(updated_at = ? AND id < ?)` tiebreak so pagination neither
// revisits nor skips rows.
func TestSQLiteListSessionsByBotPagedWalksSameSecondRows(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	createPagedSessionSchema(t, conn)

	botID := "22222222-2222-2222-2222-222222222222"
	sharedTS := "2026-06-19 00:00:00"
	ids := []string{
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01",
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02",
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03",
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb04",
		"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb05",
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

	// Walk pages of size 2 until the listing is exhausted; every row id must
	// appear exactly once and never be revisited.
	visited := make(map[string]int, len(ids))
	var cursor *pgsqlc.ListSessionsByBotPagedRow
	for step := 0; step < len(ids)+2; step++ {
		params := pgsqlc.ListSessionsByBotPagedParams{
			BotID:      pgBotID,
			Types:      []string{"chat"},
			LimitCount: 2,
		}
		if cursor != nil {
			params.UseCursor = true
			params.CursorUpdatedAt = pgtype.Timestamptz{Time: cursor.UpdatedAt.Time, Valid: true}
			params.CursorID = cursor.ID
		}
		page, err := queries.ListSessionsByBotPaged(ctx, params)
		if err != nil {
			t.Fatalf("page step %d: %v", step, err)
		}
		if len(page) == 0 {
			break
		}
		for _, row := range page {
			visited[row.ID.String()]++
		}
		last := page[len(page)-1]
		cursor = &last
	}
	if len(visited) != len(ids) {
		t.Fatalf("visited %d distinct rows, want %d (visited=%v)", len(visited), len(ids), visited)
	}
	for id, count := range visited {
		if count != 1 {
			t.Fatalf("row %s visited %d times, want exactly 1", id, count)
		}
	}
}

// TestSQLiteListSessionsByBotPagedSurfacesRouteJoinColumns guards the column
// ordering in scanSessionPagedRows. The query selects positional columns
// including turn-head metadata and the two LEFT JOIN'd route columns; a regression that swaps two
// fields would not fail the basic listing tests because most fields are
// independently asserted as scalars on a single row. Forcing a non-null
// route_metadata + route_conversation_type round-trip catches that class of
// bug.
func TestSQLiteListSessionsByBotPagedSurfacesRouteJoinColumns(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	createPagedSessionSchema(t, conn)

	botID := "44444444-4444-4444-4444-444444444444"
	routeID := "55555555-5555-5555-5555-555555555555"
	sessionID := "66666666-6666-6666-6666-666666666666"
	headTurnID := "77777777-7777-7777-7777-777777777777"
	forkedFromSessionID := "88888888-8888-8888-8888-888888888888"
	forkedFromTurnID := "99999999-9999-9999-9999-999999999999"
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO bot_channel_routes (id, metadata, conversation_type) VALUES (?, ?, ?)`,
		routeID, `{"channel":"telegram"}`, "group",
	); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO bot_sessions (
		  id, bot_id, route_id, type, default_head_turn_id, forked_from_session_id, forked_from_turn_id, updated_at, created_at
		) VALUES (?, ?, ?, 'chat', ?, ?, ?, '2026-06-19 12:00:00', '2026-06-19 12:00:00')`,
		sessionID, botID, routeID, headTurnID, forkedFromSessionID, forkedFromTurnID,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	st, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	queries := NewQueries(st)

	page, err := queries.ListSessionsByBotPaged(ctx, pgsqlc.ListSessionsByBotPagedParams{
		BotID:      mustUUID(t, botID),
		Types:      []string{"chat"},
		LimitCount: 10,
	})
	if err != nil {
		t.Fatalf("ListSessionsByBotPaged: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("page len = %d, want 1", len(page))
	}
	row := page[0]
	if row.ID.String() != sessionID {
		t.Fatalf("row id = %s, want %s", row.ID.String(), sessionID)
	}
	if row.RouteID.String() != routeID {
		t.Fatalf("row route_id = %s, want %s", row.RouteID.String(), routeID)
	}
	if row.DefaultHeadTurnID.String() != headTurnID {
		t.Fatalf("DefaultHeadTurnID = %s, want %s", row.DefaultHeadTurnID.String(), headTurnID)
	}
	if row.ForkedFromSessionID.String() != forkedFromSessionID {
		t.Fatalf("ForkedFromSessionID = %s, want %s", row.ForkedFromSessionID.String(), forkedFromSessionID)
	}
	if row.ForkedFromTurnID.String() != forkedFromTurnID {
		t.Fatalf("ForkedFromTurnID = %s, want %s", row.ForkedFromTurnID.String(), forkedFromTurnID)
	}
	if string(row.RouteMetadata) != `{"channel":"telegram"}` {
		t.Fatalf("RouteMetadata = %q, want telegram payload", string(row.RouteMetadata))
	}
	if !row.RouteConversationType.Valid || row.RouteConversationType.String != "group" {
		t.Fatalf("RouteConversationType = %#v, want group", row.RouteConversationType)
	}
}

func createPagedSessionSchema(t *testing.T, conn *sql.DB) {
	t.Helper()
	// Hand-rolled minimal schema rather than the full SQLite migration baseline:
	// these tests pin bind order and scan order for the SQLite-only paged shim.
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
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  runtime_metadata TEXT NOT NULL DEFAULT '{}',
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  default_head_turn_id TEXT,
  forked_from_session_id TEXT,
  forked_from_turn_id TEXT,
  parent_session_id TEXT,
  created_by_user_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);
`)
}
