package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	embeddeddb "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
)

const (
	sqliteACPRuntimeMigrationUp   = "sqlite/migrations/0027_acp_default_chat_runtime.up.sql"
	sqliteACPRuntimeMigrationDown = "sqlite/migrations/0027_acp_default_chat_runtime.down.sql"
)

// extractBotSessionsBackfillUpdate pulls the single "UPDATE bot_sessions ... ;"
// backfill statement out of the EMBEDDED migration file so the test exercises
// the real production SQL rather than a hand-copied literal. Reverting migration
// 0026's json_patch(...) back to json_object(...) therefore fails this test.
func extractBotSessionsBackfillUpdate(t *testing.T) string {
	t.Helper()
	raw, err := embeddeddb.MigrationsFS.ReadFile(sqliteACPRuntimeMigrationUp)
	if err != nil {
		t.Fatalf("read migration %s: %v", sqliteACPRuntimeMigrationUp, err)
	}
	content := string(raw)
	start := strings.Index(content, "UPDATE bot_sessions")
	if start < 0 {
		t.Fatalf("migration %s has no UPDATE bot_sessions statement", sqliteACPRuntimeMigrationUp)
	}
	rest := content[start:]
	end := strings.Index(rest, ";")
	if end < 0 {
		t.Fatalf("migration %s UPDATE bot_sessions is not terminated by ';'", sqliteACPRuntimeMigrationUp)
	}
	return rest[:end+1]
}

// TestSQLiteACPRuntimeMetadataBackfillParity exercises the ACP runtime migration's
// runtime_metadata backfill against the real modernc.org/sqlite engine the app
// uses, running the actual UPDATE statement extracted from the embedded
// migration. It guards the fix that replaced json_object(...) with
// json_patch(...) so null-valued optional keys are dropped, matching Postgres
// jsonb_strip_nulls in migration 0101, and the legacy-owner fallback from
// metadata.runtime_owner_account_id to created_by_user_id.
func TestSQLiteACPRuntimeMetadataBackfillParity(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Mirror the post-ALTER bot_sessions shape, including the CHECK constraints
	// the migration adds, so the fixture rejects values the real schema would.
	execAll(t, conn, `
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  type TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_by_user_id TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat' CHECK (session_mode IN ('chat', 'discuss', 'heartbeat', 'schedule', 'subagent')),
  runtime_type TEXT NOT NULL DEFAULT 'model' CHECK (runtime_type IN ('model', 'acp_agent')),
  runtime_metadata TEXT NOT NULL DEFAULT '{}'
);`)

	// Legacy rows still at the pre-split defaults: these match the backfill WHERE
	// guard (session_mode='chat' AND runtime_type='model' AND runtime_metadata='{}')
	// and must be transformed.
	legacy := []struct{ id, typ, metadata, createdBy string }{
		{"s-full", "acp_agent", `{"acp_agent_id":"codex","project_path":"/data/app","acp_project_mode":"project","runtime_owner_account_id":"owner-1"}`, "creator-ignored"},
		{"s-owner-missing", "acp_agent", `{"acp_agent_id":"codex"}`, "creator-owner"},
		{"s-owner-empty", "acp_agent", `{"acp_agent_id":"claude-code","runtime_owner_account_id":""}`, "creator-empty-owner"},
		{"s-owner-none", "acp_agent", `{"acp_agent_id":"codex"}`, ""},
		{"s-model", "chat", `{}`, "creator-model"},
	}
	for _, r := range legacy {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO bot_sessions (id, bot_id, type, metadata, created_by_user_id) VALUES (?, ?, ?, ?, NULLIF(?, ''))`,
			r.id, "bot-1", r.typ, r.metadata, r.createdBy); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}
	// Sentinel rows the WHERE guard must EXCLUDE. Each violates exactly ONE guard
	// condition so that weakening either condition (not just removing the whole
	// WHERE) is caught:
	//   s-preset        : runtime_type='acp_agent' AND non-empty runtime_metadata (already backfilled)
	//   s-guard-runtime : runtime_type='acp_agent' only   -> guards `runtime_type = 'model'`
	//   s-guard-meta    : non-empty runtime_metadata only -> guards `runtime_metadata = '{}'`
	// All must be left exactly as inserted.
	const presetMeta = `{"acp_agent_id":"preset","project_path":"/custom"}`
	excluded := []struct{ id, typ, metadata, sessionMode, runtimeType, runtimeMetadata string }{
		{"s-preset", "acp_agent", `{"acp_agent_id":"preset"}`, "chat", "acp_agent", presetMeta},
		{"s-guard-runtime", "acp_agent", `{"acp_agent_id":"sentinel"}`, "chat", "acp_agent", "{}"},
		{"s-guard-meta", "chat", "{}", "chat", "model", `{"sentinel":1}`},
	}
	for _, r := range excluded {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO bot_sessions (id, bot_id, type, metadata, session_mode, runtime_type, runtime_metadata) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.id, "bot-1", r.typ, r.metadata, r.sessionMode, r.runtimeType, r.runtimeMetadata); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	// Run the ACTUAL migration backfill statement. A second run must be a no-op
	// (transformed rows fall out of the WHERE guard), so running twice also
	// asserts re-application is safe.
	update := extractBotSessionsBackfillUpdate(t)
	if !strings.Contains(update, "json_patch") {
		t.Fatalf("migration %s backfill must use json_patch", sqliteACPRuntimeMigrationUp)
	}
	execAll(t, conn, update)
	execAll(t, conn, update)

	get := func(id string) (mode, runtime string, meta map[string]any) {
		var rawMeta string
		if err := conn.QueryRowContext(ctx,
			`SELECT session_mode, runtime_type, runtime_metadata FROM bot_sessions WHERE id = ?`, id,
		).Scan(&mode, &runtime, &rawMeta); err != nil {
			t.Fatalf("select %s: %v", id, err)
		}
		if err := json.Unmarshal([]byte(rawMeta), &meta); err != nil {
			t.Fatalf("unmarshal runtime_metadata for %s (%q): %v", id, rawMeta, err)
		}
		return mode, runtime, meta
	}
	hasKey := func(m map[string]any, key string) bool { _, ok := m[key]; return ok }

	// Fully-populated row keeps all four keys.
	mode, runtime, meta := get("s-full")
	if mode != "chat" || runtime != "acp_agent" {
		t.Fatalf("s-full descriptor = (%q,%q), want (chat,acp_agent)", mode, runtime)
	}
	if meta["acp_agent_id"] != "codex" || meta["project_path"] != "/data/app" ||
		meta["acp_project_mode"] != "project" || meta["runtime_owner_account_id"] != "owner-1" {
		t.Fatalf("s-full runtime_metadata = %#v", meta)
	}
	if len(meta) != 4 {
		t.Fatalf("s-full key count = %d, want 4 (%#v)", len(meta), meta)
	}

	// Owner-missing row: legacy ACP sessions did not always store the runtime
	// owner in metadata, so migration falls back to created_by_user_id.
	mode, runtime, meta = get("s-owner-missing")
	if mode != "chat" || runtime != "acp_agent" {
		t.Fatalf("s-owner-missing descriptor = (%q,%q), want (chat,acp_agent)", mode, runtime)
	}
	if meta["runtime_owner_account_id"] != "creator-owner" ||
		meta["acp_agent_id"] != "codex" || meta["project_path"] != "/data" || meta["acp_project_mode"] != "project" {
		t.Fatalf("s-owner-missing runtime_metadata = %#v", meta)
	}
	if len(meta) != 4 {
		t.Fatalf("s-owner-missing key count = %d, want 4 (%#v)", len(meta), meta)
	}

	// Explicit empty owner string also falls back to created_by_user_id; the
	// agent id survives.
	_, _, meta = get("s-owner-empty")
	if meta["runtime_owner_account_id"] != "creator-empty-owner" {
		t.Fatalf("s-owner-empty runtime_owner_account_id = %#v, want creator-empty-owner", meta["runtime_owner_account_id"])
	}
	if meta["acp_agent_id"] != "claude-code" || len(meta) != 4 {
		t.Fatalf("s-owner-empty runtime_metadata = %#v, want 4 keys with acp_agent_id=claude-code", meta)
	}

	// If both metadata owner and created_by_user_id are absent, the key must
	// still be absent rather than becoming null or an empty string.
	_, _, meta = get("s-owner-none")
	if hasKey(meta, "runtime_owner_account_id") {
		t.Fatalf("s-owner-none must DROP runtime_owner_account_id, got %#v", meta)
	}
	if meta["acp_agent_id"] != "codex" || len(meta) != 3 {
		t.Fatalf("s-owner-none runtime_metadata = %#v, want 3 keys without owner", meta)
	}

	// Non-ACP row stays a plain model session with an empty runtime_metadata.
	mode, runtime, meta = get("s-model")
	if mode != "chat" || runtime != "model" {
		t.Fatalf("s-model descriptor = (%q,%q), want (chat,model)", mode, runtime)
	}
	if len(meta) != 0 {
		t.Fatalf("s-model runtime_metadata = %#v, want empty", meta)
	}

	// Every excluded row must be left exactly as inserted — guarding each WHERE
	// condition individually plus re-application safety.
	mode, runtime, meta = get("s-preset")
	if mode != "chat" || runtime != "acp_agent" || meta["acp_agent_id"] != "preset" || meta["project_path"] != "/custom" || len(meta) != 2 {
		t.Fatalf("s-preset changed: (%q,%q) %#v", mode, runtime, meta)
	}
	// Excluded only by runtime_type='acp_agent'; guards the `runtime_type = 'model'` condition.
	mode, runtime, meta = get("s-guard-runtime")
	if mode != "chat" || runtime != "acp_agent" || len(meta) != 0 {
		t.Fatalf("s-guard-runtime changed (runtime_type='model' guard may be weakened): (%q,%q) %#v", mode, runtime, meta)
	}
	// Excluded only by non-empty runtime_metadata; guards the `runtime_metadata = '{}'` condition.
	mode, runtime, meta = get("s-guard-meta")
	if mode != "chat" || runtime != "model" || meta["sentinel"] == nil || len(meta) != 1 {
		t.Fatalf("s-guard-meta changed (runtime_metadata='{}' guard may be weakened): (%q,%q) %#v", mode, runtime, meta)
	}
}

func TestSQLiteACPRuntimeDownMigrationAllowsLegacyACP(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bots (
  id TEXT PRIMARY KEY,
  chat_runtime TEXT NOT NULL DEFAULT 'model',
  chat_acp_agent_id TEXT,
  chat_acp_project_path TEXT NOT NULL DEFAULT '/data',
  chat_acp_project_mode TEXT NOT NULL DEFAULT 'project'
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  runtime_metadata TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model'
);
CREATE TABLE bot_session_discuss_cursors (
  route_id TEXT
);
CREATE INDEX idx_bot_session_discuss_cursors_route
  ON bot_session_discuss_cursors(route_id)
  WHERE route_id IS NOT NULL;
CREATE INDEX idx_bot_sessions_bot_mode_runtime_active_updated
  ON bot_sessions(id, session_mode, runtime_type);
INSERT INTO bots(id, chat_runtime) VALUES('bot-1', 'model');
INSERT INTO bot_sessions(id, type, metadata, session_mode, runtime_type, runtime_metadata)
VALUES(
  's-legacy-acp',
  'acp_agent',
  '{"acp_agent_id":"codex"}',
  'chat',
  'acp_agent',
  '{"project_path":"/data/app","runtime_owner_account_id":"owner-1"}'
);`)

	raw, err := embeddeddb.MigrationsFS.ReadFile(sqliteACPRuntimeMigrationDown)
	if err != nil {
		t.Fatalf("read migration %s: %v", sqliteACPRuntimeMigrationDown, err)
	}
	execAll(t, conn, string(raw))

	var rawMeta string
	if err := conn.QueryRowContext(ctx, `SELECT metadata FROM bot_sessions WHERE id = 's-legacy-acp'`).Scan(&rawMeta); err != nil {
		t.Fatalf("select downgraded metadata: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(rawMeta), &meta); err != nil {
		t.Fatalf("unmarshal downgraded metadata %q: %v", rawMeta, err)
	}
	if meta["acp_agent_id"] != "codex" || meta["project_path"] != "/data/app" || meta["runtime_owner_account_id"] != "owner-1" {
		t.Fatalf("downgraded metadata = %#v", meta)
	}
	if hasColumn(t, conn, "bot_sessions", "runtime_type") {
		t.Fatal("bot_sessions.runtime_type still exists after down migration")
	}
}

func TestSQLiteACPRuntimeDownMigrationRejectsSplitOnlyACP(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bots (
  id TEXT PRIMARY KEY,
  chat_runtime TEXT NOT NULL DEFAULT 'model',
  chat_acp_agent_id TEXT,
  chat_acp_project_path TEXT NOT NULL DEFAULT '/data',
  chat_acp_project_mode TEXT NOT NULL DEFAULT 'project'
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  runtime_metadata TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model'
);
INSERT INTO bots(id, chat_runtime) VALUES('bot-1', 'model');
INSERT INTO bot_sessions(id, type, metadata, session_mode, runtime_type, runtime_metadata)
VALUES('s-discuss-acp', 'discuss', '{}', 'discuss', 'acp_agent', '{}');`)

	raw, err := embeddeddb.MigrationsFS.ReadFile(sqliteACPRuntimeMigrationDown)
	if err != nil {
		t.Fatalf("read migration %s: %v", sqliteACPRuntimeMigrationDown, err)
	}
	if _, err := conn.ExecContext(ctx, string(raw)); err == nil {
		t.Fatal("down migration succeeded with split-only ACP runtime; want guard failure")
	}
	if !hasColumn(t, conn, "bot_sessions", "runtime_type") {
		t.Fatal("guard failure should leave bot_sessions.runtime_type intact")
	}
}

func hasColumn(
	t *testing.T,
	conn interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	table, column string,
) bool {
	t.Helper()
	rows, err := conn.QueryContext(context.Background(), `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("table info for %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info for %s: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info for %s: %v", table, err)
	}
	return false
}
