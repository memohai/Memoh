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
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  runtime_metadata TEXT NOT NULL DEFAULT '{}',
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
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  model_id TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  request_message_id TEXT,
  assistant_message_id TEXT,
  superseded_at TEXT
);
CREATE VIEW bot_visible_history_messages AS
SELECT
  t.id AS turn_id,
  t.position AS turn_position,
  1 AS turn_message_seq,
  m.*
FROM bot_history_turns t
JOIN bot_history_messages m ON m.id = t.request_message_id
WHERE t.superseded_at IS NULL
UNION ALL
SELECT
  t.id AS turn_id,
  t.position AS turn_position,
  2 AS turn_message_seq,
  m.*
FROM bot_history_turns t
JOIN bot_history_messages m ON m.id = t.assistant_message_id
WHERE t.superseded_at IS NULL;
`)

	botID := "00000000-0000-0000-0000-000000000001"
	sessionID := "00000000-0000-0000-0000-000000000002"
	discussSessionID := "00000000-0000-0000-0000-000000000008"
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
	_, err = conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, type, session_mode, runtime_type, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, discussSessionID, botID, "discuss", "discuss", "acp_agent", "2026-05-01 01:00:00")
	if err != nil {
		t.Fatalf("insert discuss session: %v", err)
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, usage, model_id, session_mode, runtime_type, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"00000000-0000-0000-0000-000000000009",
		botID,
		discussSessionID,
		"assistant",
		`{"role":"assistant","content":"discuss"}`,
		`{"inputTokens":7,"outputTokens":4}`,
		modelID,
		"discuss",
		"acp_agent",
		"2026-05-01 02:00:00",
	)
	if err != nil {
		t.Fatalf("insert discuss usage: %v", err)
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
	_, err = conn.ExecContext(ctx, `INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, metadata, usage, model_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"00000000-0000-0000-0000-000000000010",
		botID,
		sessionID,
		"user",
		`{"role":"user","content":"use beta too"}`,
		`{"model_requested_skills":[{"name":"beta"},{"name":"alpha"}]}`,
		"",
		nil,
		"2026-05-01 01:02:00",
	)
	if err != nil {
		t.Fatalf("insert requested skill metadata: %v", err)
	}
	for _, item := range []struct {
		id        string
		sessionID string
		position  int
		requestID string
		replyID   string
	}{
		{"00000000-0000-0000-0000-000000000105", sessionID, 1, "", "00000000-0000-0000-0000-000000000005"},
		{"00000000-0000-0000-0000-000000000106", sessionID, 2, "00000000-0000-0000-0000-000000000006", ""},
		{"00000000-0000-0000-0000-000000000110", sessionID, 3, "00000000-0000-0000-0000-000000000010", ""},
		{"00000000-0000-0000-0000-000000000109", discussSessionID, 1, "", "00000000-0000-0000-0000-000000000009"},
	} {
		_, err = conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, session_id, position, request_message_id, assistant_message_id) VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''))`,
			item.id,
			botID,
			item.sessionID,
			item.position,
			item.requestID,
			item.replyID,
		)
		if err != nil {
			t.Fatalf("insert history turn %s: %v", item.id, err)
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
	if len(rows) != 2 {
		t.Fatalf("usage row count = %d, want 2", len(rows))
	}
	usageByType := map[string]pgsqlc.GetTokenUsageByDayAndTypeRow{}
	for _, row := range rows {
		usageByType[row.SessionType] = row
	}
	chatUsage := usageByType["chat"]
	if chatUsage.InputTokens != 10 || chatUsage.OutputTokens != 5 || chatUsage.CacheReadTokens != 3 || chatUsage.ReasoningTokens != 1 {
		t.Fatalf("chat usage row = %+v, want token totals", chatUsage)
	}
	if !chatUsage.Day.Valid || chatUsage.Day.Time.Format("2006-01-02") != "2026-05-01" {
		t.Fatalf("usage day = %+v, want 2026-05-01", chatUsage.Day)
	}
	acpUsage := usageByType["acp_agent"]
	if acpUsage.InputTokens != 7 || acpUsage.OutputTokens != 4 {
		t.Fatalf("ACP usage row = %+v, want ACP runtime totals", acpUsage)
	}
	chatRows, err := q.GetTokenUsageByDayAndType(ctx, pgsqlc.GetTokenUsageByDayAndTypeParams{
		BotID:       mustUUID(t, botID),
		FromTime:    from,
		ToTime:      to,
		SessionType: pgtype.Text{String: "chat", Valid: true},
	})
	if err != nil {
		t.Fatalf("chat usage by day: %v", err)
	}
	if len(chatRows) != 1 || chatRows[0].SessionType != "chat" || chatRows[0].InputTokens != 10 {
		t.Fatalf("chat-filtered usage rows = %+v, want only model chat usage", chatRows)
	}
	discussRows, err := q.GetTokenUsageByDayAndType(ctx, pgsqlc.GetTokenUsageByDayAndTypeParams{
		BotID:       mustUUID(t, botID),
		FromTime:    from,
		ToTime:      to,
		SessionType: pgtype.Text{String: "discuss", Valid: true},
	})
	if err != nil {
		t.Fatalf("discuss usage by day: %v", err)
	}
	if len(discussRows) != 0 {
		t.Fatalf("discuss-filtered usage rows = %+v, want ACP discuss excluded from model discuss", discussRows)
	}
	acpRows, err := q.GetTokenUsageByDayAndType(ctx, pgsqlc.GetTokenUsageByDayAndTypeParams{
		BotID:       mustUUID(t, botID),
		FromTime:    from,
		ToTime:      to,
		SessionType: pgtype.Text{String: "acp_agent", Valid: true},
	})
	if err != nil {
		t.Fatalf("ACP usage by day: %v", err)
	}
	if len(acpRows) != 1 || acpRows[0].SessionType != "acp_agent" || acpRows[0].InputTokens != 7 {
		t.Fatalf("ACP-filtered usage rows = %+v, want only ACP runtime usage", acpRows)
	}

	skills, err := q.GetSessionUsedSkills(ctx, mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("used skills: %v", err)
	}
	if len(skills) != 2 || skills[0] != "alpha" || skills[1] != "beta" {
		t.Fatalf("skills = %#v, want [alpha beta]", skills)
	}
}

func TestSQLiteMCPOAuthScopesSupportedRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE mcp_connections (
  id TEXT PRIMARY KEY
);
CREATE TABLE mcp_oauth_tokens (
  id TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL UNIQUE REFERENCES mcp_connections(id) ON DELETE CASCADE,
  resource_metadata_url TEXT NOT NULL DEFAULT '',
  authorization_server_url TEXT NOT NULL DEFAULT '',
  authorization_endpoint TEXT NOT NULL DEFAULT '',
  token_endpoint TEXT NOT NULL DEFAULT '',
  registration_endpoint TEXT NOT NULL DEFAULT '',
  scopes_supported TEXT NOT NULL DEFAULT '{}',
  client_id TEXT NOT NULL DEFAULT '',
  client_secret TEXT NOT NULL DEFAULT '',
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT 'Bearer',
  expires_at TEXT,
  scope TEXT NOT NULL DEFAULT '',
  pkce_code_verifier TEXT NOT NULL DEFAULT '',
  state_param TEXT NOT NULL DEFAULT '',
  resource_uri TEXT NOT NULL DEFAULT '',
  redirect_uri TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	connectionID := "00000000-0000-0000-0000-000000000101"
	if _, err := conn.ExecContext(ctx, `INSERT INTO mcp_connections (id) VALUES (?)`, connectionID); err != nil {
		t.Fatalf("insert connection: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	created, err := q.UpsertMCPOAuthDiscovery(ctx, pgsqlc.UpsertMCPOAuthDiscoveryParams{ //nolint:gosec // Test OAuth URLs are not credentials.
		ConnectionID:           mustUUID(t, connectionID),
		ResourceMetadataUrl:    "https://example.test/.well-known/oauth-protected-resource",
		AuthorizationServerUrl: "https://auth.example.test",
		AuthorizationEndpoint:  "https://auth.example.test/authorize",
		TokenEndpoint:          "https://auth.example.test/token",
		RegistrationEndpoint:   "https://auth.example.test/register",
		ScopesSupported:        []string{"openid", "profile"},
		ResourceUri:            "https://example.test/mcp",
	})
	if err != nil {
		t.Fatalf("upsert oauth discovery: %v", err)
	}
	assertScopes(t, created.ScopesSupported, []string{"openid", "profile"})

	var storedScopes string
	if err := conn.QueryRowContext(ctx, `SELECT scopes_supported FROM mcp_oauth_tokens WHERE connection_id = ?`, connectionID).Scan(&storedScopes); err != nil {
		t.Fatalf("read stored scopes: %v", err)
	}
	if storedScopes != `["openid","profile"]` {
		t.Fatalf("stored scopes = %q, want JSON array", storedScopes)
	}

	loaded, err := q.GetMCPOAuthToken(ctx, mustUUID(t, connectionID))
	if err != nil {
		t.Fatalf("get oauth token: %v", err)
	}
	assertScopes(t, loaded.ScopesSupported, []string{"openid", "profile"})

	if _, err := conn.ExecContext(ctx, `UPDATE mcp_oauth_tokens SET scopes_supported = ? WHERE connection_id = ?`, "[email offline_access]", connectionID); err != nil {
		t.Fatalf("write legacy scopes: %v", err)
	}
	legacy, err := q.GetMCPOAuthToken(ctx, mustUUID(t, connectionID))
	if err != nil {
		t.Fatalf("get legacy oauth token: %v", err)
	}
	assertScopes(t, legacy.ScopesSupported, []string{"email", "offline_access"})
}

func TestSQLiteUserInputReusesRepeatedToolCallID(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bots (
  id TEXT PRIMARY KEY
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY
);
CREATE TABLE bot_channel_routes (
  id TEXT PRIMARY KEY
);
CREATE TABLE channel_identities (
  id TEXT PRIMARY KEY
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY
);
CREATE TABLE user_input_requests (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL DEFAULT 'ask_user',
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  input_json TEXT NOT NULL,
  ui_payload_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}',
  provider_metadata TEXT NOT NULL DEFAULT '{}',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  responded_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  assistant_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  tool_result_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  expires_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at TEXT,
  canceled_at TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	  CONSTRAINT user_input_tool_name_check CHECK (tool_name = 'ask_user'),
	  CONSTRAINT user_input_status_check CHECK (status IN ('pending', 'submitted', 'canceled', 'expired', 'failed')),
	  CONSTRAINT user_input_short_id_unique UNIQUE (session_id, short_id)
	);
	CREATE UNIQUE INDEX user_input_tool_call_unique
	  ON user_input_requests(session_id, tool_call_id);
	`)

	botID := "00000000-0000-0000-0000-000000001001"
	sessionID := "00000000-0000-0000-0000-000000001002"
	actorID := "00000000-0000-0000-0000-000000001003"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bots (id) VALUES (?)`, botID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id) VALUES (?)`, sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO channel_identities (id) VALUES (?)`, actorID); err != nil {
		t.Fatalf("insert identity: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)
	create := func(question string) pgsqlc.UserInputRequest {
		t.Helper()
		row, err := q.CreateUserInputRequest(ctx, pgsqlc.CreateUserInputRequestParams{
			BotID:                        mustUUID(t, botID),
			SessionID:                    mustUUID(t, sessionID),
			ToolCallID:                   "reused-call-id",
			ToolName:                     "ask_user",
			InputJson:                    []byte(`{"question":"` + question + `"}`),
			UiPayloadJson:                []byte(`{"question":"` + question + `"}`),
			ProviderMetadata:             []byte(`{}`),
			RequestedByChannelIdentityID: mustUUID(t, actorID),
		})
		if err != nil {
			t.Fatalf("create user input %q: %v", question, err)
		}
		return row
	}

	first := create("first")
	if first.Status != "pending" || first.ShortID != 1 {
		t.Fatalf("first request = status %q short_id %d", first.Status, first.ShortID)
	}
	updated := create("second")
	if updated.ID != first.ID {
		t.Fatalf("pending reused tool_call_id created request %s, want existing %s", updated.ID, first.ID)
	}
	if updated.Status != "pending" || updated.ShortID != 1 {
		t.Fatalf("updated request = status %q short_id %d, want pending #1", updated.Status, updated.ShortID)
	}

	canceled, err := q.CancelUserInputRequest(ctx, pgsqlc.CancelUserInputRequestParams{
		ID:                           updated.ID,
		ResultJson:                   []byte(`{"status":"canceled"}`),
		RespondedByChannelIdentityID: mustUUID(t, actorID),
	})
	if err != nil {
		t.Fatalf("cancel first: %v", err)
	}
	if canceled.Status != "canceled" {
		t.Fatalf("canceled status = %q", canceled.Status)
	}

	terminal, err := q.CreateUserInputRequest(ctx, pgsqlc.CreateUserInputRequestParams{
		BotID:                        mustUUID(t, botID),
		SessionID:                    mustUUID(t, sessionID),
		ToolCallID:                   "reused-call-id",
		ToolName:                     "ask_user",
		InputJson:                    []byte(`{"question":"third"}`),
		UiPayloadJson:                []byte(`{"question":"third"}`),
		ProviderMetadata:             []byte(`{}`),
		RequestedByChannelIdentityID: mustUUID(t, actorID),
	})
	if err == nil {
		if terminal.ID != first.ID || terminal.Status != "canceled" || terminal.ShortID != 1 || string(terminal.InputJson) == `{"question":"third"}` {
			t.Fatalf("terminal duplicate returned %#v, want unchanged canceled original", terminal)
		}
	}
	existing, err := q.GetUserInputRequestBySessionToolCall(ctx, pgsqlc.GetUserInputRequestBySessionToolCallParams{
		SessionID:  mustUUID(t, sessionID),
		ToolCallID: "reused-call-id",
	})
	if err != nil {
		t.Fatalf("get existing terminal request: %v", err)
	}
	if existing.ID != first.ID || existing.Status != "canceled" || existing.ShortID != 1 {
		t.Fatalf("existing request = %#v, want canceled original", existing)
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

func assertScopes(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("scopes = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes = %#v, want %#v", got, want)
		}
	}
}
