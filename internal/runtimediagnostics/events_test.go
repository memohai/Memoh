package runtimediagnostics

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
)

func TestSanitizeEventMetadataRedactsSensitiveValues(t *testing.T) {
	input := map[string]any{
		"api_key": "sk-secret",
		"agent":   "codex",
		"nested": map[string]any{
			"oauth_token": "oauth-secret",
			"base_url":    "https://api.example.test",
			"stderr":      "failed with Authorization: Bearer nested-secret",
		},
		"items": []any{
			map[string]any{"refresh_token": "refresh-secret"},
			"plain api_key=item-secret",
		},
		"lines": []string{
			"ok",
			"token=line-secret",
		},
	}

	got := SanitizeEventMetadata(input)

	if got["api_key"] != "[redacted]" {
		t.Fatalf("api_key = %#v, want redacted", got["api_key"])
	}
	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested metadata = %#v, want map", got["nested"])
	}
	if nested["oauth_token"] != "[redacted]" {
		t.Fatalf("oauth_token = %#v, want redacted", nested["oauth_token"])
	}
	if nested["base_url"] != "https://api.example.test" {
		t.Fatalf("base_url = %#v, want preserved", nested["base_url"])
	}
	if strings.Contains(nested["stderr"].(string), "nested-secret") {
		t.Fatalf("nested stderr leaked secret in %#v", nested["stderr"])
	}
	items, ok := got["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v, want two sanitized entries", got["items"])
	}
	item0, ok := items[0].(map[string]any)
	if !ok || item0["refresh_token"] != "[redacted]" {
		t.Fatalf("items[0] = %#v, want refresh_token redacted", items[0])
	}
	if strings.Contains(items[1].(string), "item-secret") {
		t.Fatalf("items[1] leaked secret in %#v", items[1])
	}
	lines, ok := got["lines"].([]any)
	if !ok || len(lines) != 2 {
		t.Fatalf("lines = %#v, want two sanitized entries", got["lines"])
	}
	if strings.Contains(lines[1].(string), "line-secret") {
		t.Fatalf("lines[1] leaked secret in %#v", lines[1])
	}
}

func TestSanitizeEventMessageRedactsSensitiveValues(t *testing.T) {
	input := `runtime failed: api_key=sk-secret Authorization: Bearer oauth-secret refresh_token:"refresh-secret" {"access_token":"json-secret"} keep=this`

	got := SanitizeEventMessage(input)

	for _, leaked := range []string{"sk-secret", "oauth-secret", "refresh-secret", "json-secret"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("sanitized message leaked %q in %q", leaked, got)
		}
	}
	for _, expected := range []string{"api_key=[redacted]", "Authorization: [redacted]", "refresh_token:[redacted]", `"access_token":[redacted]`, "keep=this"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("sanitized message = %q, want %q", got, expected)
		}
	}
}

func TestRecorderPersistsSanitizedRecentEventsWithIsolationAndRetention(t *testing.T) {
	ctx := context.Background()
	conn, queries := newRuntimeDiagnosticEventTestDB(t)
	botID := "11111111-1111-1111-1111-111111111111"
	otherBotID := "22222222-2222-2222-2222-222222222222"
	insertRuntimeDiagnosticBot(t, conn, botID)
	insertRuntimeDiagnosticBot(t, conn, otherBotID)
	insertOldRuntimeDiagnosticEvent(t, conn, botID)

	recorder := NewRecorder(queries)
	recorder.now = func() time.Time { return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC) }
	recorder.perBotLimit = 1

	if err := recorder.Record(ctx, EventInput{
		BotID:    botID,
		Scope:    "unknown-scope",
		Severity: "bad-severity",
		Message:  "start failed api_key=sk-secret Authorization: Bearer oauth-secret",
		Metadata: map[string]any{
			"api_key": "sk-secret",
			"stderr":  "failed refresh_token=refresh-secret",
			"nested": map[string]any{
				"access_token": "access-secret",
			},
		},
	}); err != nil {
		t.Fatalf("Record() bot event error = %v", err)
	}
	if err := recorder.Record(ctx, EventInput{
		BotID:    otherBotID,
		Scope:    "display",
		Severity: "warn",
		Code:     "display_unavailable",
		Message:  "other bot event",
	}); err != nil {
		t.Fatalf("Record() other bot event error = %v", err)
	}

	events, err := recorder.ListRecent(ctx, botID, 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1 after per-bot prune and retention; events=%#v", len(events), events)
	}
	event := events[0]
	if event.BotID != botID {
		t.Fatalf("event bot_id = %q, want %q", event.BotID, botID)
	}
	if event.Scope != "acp" {
		t.Fatalf("scope = %q, want normalized acp", event.Scope)
	}
	if event.Severity != "error" {
		t.Fatalf("severity = %q, want normalized error", event.Severity)
	}
	if event.Phase != "runtime" {
		t.Fatalf("phase = %q, want default runtime", event.Phase)
	}
	if event.Code != "runtime_diagnostic_event" {
		t.Fatalf("code = %q, want default runtime_diagnostic_event", event.Code)
	}
	for _, leaked := range []string{"sk-secret", "oauth-secret", "refresh-secret", "access-secret"} {
		if strings.Contains(event.Message, leaked) {
			t.Fatalf("event message leaked %q in %q", leaked, event.Message)
		}
		if strings.Contains(anyMapString(event.Metadata), leaked) {
			t.Fatalf("event metadata leaked %q in %#v", leaked, event.Metadata)
		}
	}

	otherEvents, err := recorder.ListRecent(ctx, otherBotID, 10)
	if err != nil {
		t.Fatalf("ListRecent() other bot error = %v", err)
	}
	if len(otherEvents) != 1 || otherEvents[0].BotID != otherBotID {
		t.Fatalf("other bot events = %#v, want isolated event", otherEvents)
	}
}

func TestRecorderPruneRunsRetentionAndPerBotLimitWithoutRecording(t *testing.T) {
	ctx := context.Background()
	conn, queries := newRuntimeDiagnosticEventTestDB(t)
	botID := "11111111-1111-1111-1111-111111111111"
	otherBotID := "22222222-2222-2222-2222-222222222222"
	insertRuntimeDiagnosticBot(t, conn, botID)
	insertRuntimeDiagnosticBot(t, conn, otherBotID)
	insertRuntimeDiagnosticEventAt(t, conn, botID, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "old_event", "2026-04-01 00:00:00")
	insertRuntimeDiagnosticEventAt(t, conn, botID, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "newer_event", "2026-06-23 11:00:00")
	insertRuntimeDiagnosticEventAt(t, conn, botID, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "newest_event", "2026-06-23 12:00:00")
	insertRuntimeDiagnosticEventAt(t, conn, otherBotID, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "other_event", "2026-06-23 12:00:00")

	recorder := NewRecorder(queries)
	recorder.now = func() time.Time { return time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC) }
	recorder.perBotLimit = 1

	if err := recorder.Prune(ctx); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	events, err := recorder.ListRecent(ctx, botID, 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(events) != 1 || events[0].Code != "newest_event" {
		t.Fatalf("bot events after prune = %#v, want only newest_event", events)
	}
	otherEvents, err := recorder.ListRecent(ctx, otherBotID, 10)
	if err != nil {
		t.Fatalf("ListRecent() other bot error = %v", err)
	}
	if len(otherEvents) != 1 || otherEvents[0].Code != "other_event" {
		t.Fatalf("other bot events after prune = %#v, want other_event", otherEvents)
	}
}

func TestRecordRuntimeDiagnosticEventPersistsAfterCallerContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, queries := newRuntimeDiagnosticEventTestDB(t)
	botID := "11111111-1111-1111-1111-111111111111"
	insertRuntimeDiagnosticBot(t, conn, botID)

	recorder := NewRecorder(queries)
	recorder.RecordRuntimeDiagnosticEvent(ctx, botID, "acp", "codex", "", "runtime-1", "start", "error", "runtime_start_failed", "context cancelled after failure", nil)

	events, err := recorder.ListRecent(context.Background(), botID, 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].Code != "runtime_start_failed" || events[0].RuntimeID != "runtime-1" {
		t.Fatalf("event = %#v, want runtime_start_failed for runtime-1", events[0])
	}
}

func newRuntimeDiagnosticEventTestDB(t *testing.T) (*sql.DB, *sqlitestore.Queries) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	execRuntimeDiagnosticSQL(t, conn, `
CREATE TABLE bots (
  id TEXT PRIMARY KEY
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY
);
CREATE TABLE runtime_diagnostic_events (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  scope TEXT NOT NULL CHECK (scope IN ('workspace', 'container', 'display', 'acp')),
  agent_id TEXT NOT NULL DEFAULT '',
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  runtime_id TEXT NOT NULL DEFAULT '',
  phase TEXT NOT NULL,
  severity TEXT NOT NULL CHECK (severity IN ('info', 'warn', 'error')),
  code TEXT NOT NULL,
  message TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_runtime_diagnostic_events_bot_created
  ON runtime_diagnostic_events(bot_id, created_at DESC);
CREATE INDEX idx_runtime_diagnostic_events_bot_scope_created
  ON runtime_diagnostic_events(bot_id, scope, created_at DESC);
`)
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, sqlitestore.NewQueries(store)
}

func insertRuntimeDiagnosticBot(t *testing.T, conn *sql.DB, id string) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), `INSERT INTO bots (id) VALUES (?)`, id); err != nil {
		t.Fatalf("insert bot %s: %v", id, err)
	}
}

func insertOldRuntimeDiagnosticEvent(t *testing.T, conn *sql.DB, botID string) {
	t.Helper()
	insertRuntimeDiagnosticEventAt(t, conn, botID, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "old_event", "2026-04-01 00:00:00")
}

func insertRuntimeDiagnosticEventAt(t *testing.T, conn *sql.DB, botID, id, code, createdAt string) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), `
INSERT INTO runtime_diagnostic_events (
  id, bot_id, scope, phase, severity, code, message, metadata, created_at
) VALUES (
  ?, ?, 'acp', 'runtime', 'error', ?, ?, '{}', ?
)`, id, botID, code, code, createdAt)
	if err != nil {
		t.Fatalf("insert runtime diagnostic event %s: %v", code, err)
	}
}

func execRuntimeDiagnosticSQL(t *testing.T, conn *sql.DB, statement string) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), statement); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}

func anyMapString(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		var b strings.Builder
		for key, val := range typed {
			b.WriteString(key)
			b.WriteByte('=')
			b.WriteString(anyMapString(val))
			b.WriteByte(';')
		}
		return b.String()
	case []any:
		var b strings.Builder
		for _, item := range typed {
			b.WriteString(anyMapString(item))
			b.WriteByte(';')
		}
		return b.String()
	case string:
		return typed
	default:
		return ""
	}
}
