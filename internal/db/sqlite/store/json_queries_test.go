package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteJSONUsageAndSkillQueries(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL
);
CREATE TABLE models (
  id TEXT PRIMARY KEY,
  model_id TEXT NOT NULL,
  name TEXT,
  provider_id TEXT NOT NULL REFERENCES providers(id)
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  type TEXT NOT NULL,
  parent_session_id TEXT,
  deleted_at TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  model_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000000001"
	sessionID := "00000000-0000-0000-0000-000000000002"
	modelID := "00000000-0000-0000-0000-000000000003"
	providerID := "00000000-0000-0000-0000-000000000004"
	_, err = conn.ExecContext(ctx, `INSERT INTO providers (id, name) VALUES (?, ?)`, providerID, "Test Provider")
	if err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO models (id, model_id, name, provider_id) VALUES (?, ?, ?, ?)`, modelID, "test-model", "Test Model", providerID)
	if err != nil {
		t.Fatalf("insert model: %v", err)
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, type, updated_at) VALUES (?, ?, ?, ?)`, sessionID, botID, "chat", "2026-05-01 01:00:00")
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, usage, model_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"00000000-0000-0000-0000-000000000005",
		botID,
		sessionID,
		"assistant",
		`{"role":"assistant","content":[{"type":"tool-call","toolName":"use_skill","input":{"skillName":"alpha"}}]}`,
		`{"inputTokens":10,"outputTokens":5,"inputTokenDetails":{"cacheReadTokens":3,"cacheWriteTokens":2},"outputTokenDetails":{"reasoningTokens":1}}`,
		modelID,
		"2026-05-01 01:00:00",
	)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
	for _, item := range []struct {
		id      string
		role    string
		content string
	}{
		{
			id:      "00000000-0000-0000-0000-000000000006",
			role:    "user",
			content: `{"role":"user","content":"hello"}`,
		},
		{
			id:      "00000000-0000-0000-0000-000000000007",
			role:    "tool",
			content: `{"role":"tool","content":[{"type":"tool-result","toolName":"use_skill","result":{}}]}`,
		},
	} {
		_, err = conn.ExecContext(ctx, `INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, usage, model_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			item.role,
			item.content,
			"",
			nil,
			"2026-05-01 01:01:00",
		)
		if err != nil {
			t.Fatalf("insert empty usage %s: %v", item.role, err)
		}
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	from := pgtype.Timestamptz{Time: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	to := pgtype.Timestamptz{Time: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC), Valid: true}
	rows, err := q.GetTokenUsageByDayAndType(ctx, pgsqlc.GetTokenUsageByDayAndTypeParams{
		BotID:    mustUUID(t, botID),
		FromTime: from,
		ToTime:   to,
	})
	if err != nil {
		t.Fatalf("usage by day: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("usage row count = %d, want 1", len(rows))
	}
	if rows[0].InputTokens != 10 || rows[0].OutputTokens != 5 || rows[0].CacheReadTokens != 3 || rows[0].CacheWriteTokens != 2 || rows[0].ReasoningTokens != 1 {
		t.Fatalf("usage row = %+v, want token totals", rows[0])
	}
	if !rows[0].Day.Valid || rows[0].Day.Time.Format("2006-01-02") != "2026-05-01" {
		t.Fatalf("usage day = %+v, want 2026-05-01", rows[0].Day)
	}

	skills, err := q.GetSessionUsedSkills(ctx, mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("used skills: %v", err)
	}
	if len(skills) != 1 || skills[0] != "alpha" {
		t.Fatalf("skills = %#v, want [alpha]", skills)
	}
}

func execAll(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), statement); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}
