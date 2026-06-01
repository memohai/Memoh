package store

import (
	"context"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// TestSQLiteCommandUILanguageRoundTrip exercises the hand-edited (sqlc was
// unavailable) settings queries against a real in-memory SQLite database. It is
// the runtime guard the Go compiler cannot provide: a malformed SELECT/RETURNING
// column list, a wrong positional parameter, or a Scan-order mismatch would only
// surface here.
func TestSQLiteCommandUILanguageRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Minimal schema mirroring the columns the settings queries touch, including
	// the new command_ui_language column (NOT NULL DEFAULT 'auto'). The LEFT JOIN
	// targets must exist but can stay empty.
	execAll(t, conn, `
CREATE TABLE models (id TEXT PRIMARY KEY);
CREATE TABLE search_providers (id TEXT PRIMARY KEY);
CREATE TABLE memory_providers (id TEXT PRIMARY KEY);
CREATE TABLE bots (
  id TEXT PRIMARY KEY,
  language TEXT NOT NULL DEFAULT 'auto',
  command_ui_language TEXT NOT NULL DEFAULT 'auto',
  reasoning_enabled INTEGER NOT NULL DEFAULT 0,
  reasoning_effort TEXT NOT NULL DEFAULT 'medium',
  heartbeat_enabled INTEGER NOT NULL DEFAULT 0,
  heartbeat_interval INTEGER NOT NULL DEFAULT 30,
  heartbeat_prompt TEXT NOT NULL DEFAULT '',
  compaction_enabled INTEGER NOT NULL DEFAULT 0,
  compaction_threshold INTEGER NOT NULL DEFAULT 100000,
  compaction_ratio INTEGER NOT NULL DEFAULT 80,
  timezone TEXT,
  chat_model_id TEXT,
  heartbeat_model_id TEXT,
  compaction_model_id TEXT,
  title_model_id TEXT,
  image_model_id TEXT,
  search_provider_id TEXT,
  memory_provider_id TEXT,
  tts_model_id TEXT,
  transcription_model_id TEXT,
  persist_full_tool_results INTEGER NOT NULL DEFAULT 0,
  show_tool_calls_in_im INTEGER NOT NULL DEFAULT 0,
  tool_approval_config TEXT NOT NULL DEFAULT '{}',
  display_enabled INTEGER NOT NULL DEFAULT 0,
  overlay_provider TEXT NOT NULL DEFAULT '',
  overlay_enabled INTEGER NOT NULL DEFAULT 0,
  overlay_config TEXT NOT NULL DEFAULT '{}',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000000001"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bots (id) VALUES (?)`, botID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	// Fresh row defaults to "auto" — proves GetSettingsByBotID selects/scans the
	// new column at the right position.
	got, err := q.GetSettingsByBotID(ctx, mustUUID(t, botID))
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if got.CommandUiLanguage != "auto" {
		t.Fatalf("default CommandUiLanguage = %q, want auto", got.CommandUiLanguage)
	}

	// Upsert to "zh" — proves the SET parameter, RETURNING column, and Scan all
	// line up.
	updated, err := q.UpsertBotSettings(ctx, pgsqlc.UpsertBotSettingsParams{
		ID:                  mustUUID(t, botID),
		Language:            "en",
		CommandUiLanguage:   "zh",
		ReasoningEffort:     "medium",
		HeartbeatInterval:   30,
		HeartbeatPrompt:     "",
		CompactionThreshold: 100000,
		CompactionRatio:     80,
		ToolApprovalConfig:  []byte("{}"),
		OverlayProvider:     "",
		OverlayConfig:       []byte("{}"),
	})
	if err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	if updated.CommandUiLanguage != "zh" {
		t.Fatalf("upsert RETURNING CommandUiLanguage = %q, want zh", updated.CommandUiLanguage)
	}

	// Re-read to confirm persistence through the GET path.
	reread, err := q.GetSettingsByBotID(ctx, mustUUID(t, botID))
	if err != nil {
		t.Fatalf("re-get settings: %v", err)
	}
	if reread.CommandUiLanguage != "zh" {
		t.Fatalf("persisted CommandUiLanguage = %q, want zh", reread.CommandUiLanguage)
	}
}
