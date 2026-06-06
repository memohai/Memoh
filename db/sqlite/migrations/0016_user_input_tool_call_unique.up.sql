-- 0016_user_input_tool_call_unique
-- Make ask_user requests idempotent per session tool call.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS user_input_requests_new (
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

INSERT OR IGNORE INTO user_input_requests_new (
  id, bot_id, session_id, route_id, channel_identity_id, tool_call_id, tool_name,
  short_id, status, input_json, ui_payload_json, result_json, provider_metadata,
  requested_by_channel_identity_id, responded_by_channel_identity_id,
  assistant_message_id, tool_result_message_id, prompt_message_id,
  prompt_external_message_id, source_platform, reply_target, conversation_type,
  expires_at, created_at, responded_at, canceled_at, updated_at
)
SELECT
  id, bot_id, session_id, route_id, channel_identity_id, tool_call_id, tool_name,
  short_id, status, input_json, ui_payload_json, result_json, provider_metadata,
  requested_by_channel_identity_id, responded_by_channel_identity_id,
  assistant_message_id, tool_result_message_id, prompt_message_id,
  prompt_external_message_id, source_platform, reply_target, conversation_type,
  expires_at, created_at, responded_at, canceled_at, updated_at
FROM user_input_requests;

DROP TABLE user_input_requests;
ALTER TABLE user_input_requests_new RENAME TO user_input_requests;

DELETE FROM user_input_requests
WHERE id IN (
  SELECT id
  FROM (
    SELECT
      id,
      ROW_NUMBER() OVER (
        PARTITION BY session_id, tool_call_id
        ORDER BY
          CASE
            WHEN status IN ('submitted', 'canceled', 'failed', 'expired') THEN 0
            WHEN status = 'pending' THEN 1
            ELSE 2
          END,
          COALESCE(responded_at, canceled_at, updated_at, created_at) DESC,
          updated_at DESC,
          created_at DESC,
          id DESC
      ) AS rn
    FROM user_input_requests
  )
  WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_unique
  ON user_input_requests(session_id, tool_call_id);
CREATE INDEX IF NOT EXISTS idx_user_input_bot_status_created
  ON user_input_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_session_status_created
  ON user_input_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_prompt_external
  ON user_input_requests(prompt_external_message_id)
  WHERE prompt_external_message_id != '';

PRAGMA foreign_keys = ON;
