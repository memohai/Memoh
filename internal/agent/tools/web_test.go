package tools

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
)

func TestWebSearchProviderRejectsDisabledProvider(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	execAllSQL(t, ctx, conn, `
CREATE TABLE models (id TEXT PRIMARY KEY);
CREATE TABLE search_providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  enable INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE fetch_providers (id TEXT PRIMARY KEY);
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
  chat_runtime TEXT NOT NULL DEFAULT 'model',
  chat_acp_agent_id TEXT,
  chat_acp_project_path TEXT NOT NULL DEFAULT '/data',
  chat_acp_project_mode TEXT NOT NULL DEFAULT 'project',
  heartbeat_model_id TEXT,
  compaction_model_id TEXT,
  title_model_id TEXT,
  image_model_id TEXT,
  search_provider_id TEXT,
  fetch_provider_id TEXT,
  memory_provider_id TEXT,
  tts_model_id TEXT,
  transcription_model_id TEXT,
  video_model_id TEXT,
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

	botID := "00000000-0000-0000-0000-000000000011"
	providerID := "00000000-0000-0000-0000-000000000012"
	if _, err := conn.ExecContext(ctx, `INSERT INTO search_providers (id, name, provider, config, enable) VALUES (?, 'Disabled Brave', 'brave', '{}', 0)`, providerID); err != nil {
		t.Fatalf("insert search provider: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bots (id, search_provider_id) VALUES (?, ?)`, botID, providerID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}

	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	queries := sqlitestore.NewQueries(store)
	provider := NewWebProvider(
		slog.New(slog.DiscardHandler),
		settings.NewService(slog.New(slog.DiscardHandler), queries, nil, nil),
		searchproviders.NewService(slog.New(slog.DiscardHandler), queries),
	)

	_, err = provider.execWebSearch(ctx, SessionContext{BotID: botID}, map[string]any{"query": "memoh"})
	if err == nil || !strings.Contains(err.Error(), "search provider is disabled") {
		t.Fatalf("execWebSearch() error = %v, want disabled provider error", err)
	}
}

func execAllSQL(t *testing.T, ctx context.Context, db execer, sql string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, sql); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
