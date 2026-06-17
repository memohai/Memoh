-- 0024_pending_branch_turn_context
-- Persist branch/turn context for deferred tool approval and ask_user continuations.

PRAGMA foreign_keys = OFF;

BEGIN;

CREATE TABLE IF NOT EXISTS tool_approval_requests_0023_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  operation TEXT NOT NULL,
  tool_input TEXT NOT NULL,
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  decision_reason TEXT NOT NULL DEFAULT '',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  decided_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  requested_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  persist_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  persist_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  decided_at TEXT,
  CONSTRAINT tool_approval_operation_check CHECK (operation IN ('read', 'write', 'exec')),
  CONSTRAINT tool_approval_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
  CONSTRAINT tool_approval_short_id_unique UNIQUE (session_id, short_id),
  CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id)
);

INSERT OR IGNORE INTO tool_approval_requests_0023_new (
  id, bot_id, session_id, route_id, channel_identity_id, tool_call_id, tool_name,
  operation, tool_input, short_id, status, decision_reason, requested_by_channel_identity_id,
  decided_by_channel_identity_id, requested_message_id, prompt_message_id,
  persist_branch_id, persist_turn_id, prompt_external_message_id, source_platform,
  reply_target, conversation_type, created_at, decided_at
)
SELECT
  id, bot_id, session_id, route_id, channel_identity_id, tool_call_id, tool_name,
  operation, tool_input, short_id, status, decision_reason, requested_by_channel_identity_id,
  decided_by_channel_identity_id, requested_message_id, prompt_message_id,
  NULL, NULL,
  prompt_external_message_id, source_platform, reply_target, conversation_type,
  created_at, decided_at
FROM tool_approval_requests;

DROP TABLE tool_approval_requests;
ALTER TABLE tool_approval_requests_0023_new RENAME TO tool_approval_requests;

CREATE INDEX IF NOT EXISTS idx_tool_approval_bot_status_created
  ON tool_approval_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_session_status_created
  ON tool_approval_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_prompt_external
  ON tool_approval_requests(prompt_external_message_id)
  WHERE prompt_external_message_id != '';

CREATE TABLE IF NOT EXISTS user_input_requests_0023_new (
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
  persist_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  persist_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
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

INSERT OR IGNORE INTO user_input_requests_0023_new (
  id, bot_id, session_id, route_id, channel_identity_id, tool_call_id, tool_name,
  short_id, status, input_json, ui_payload_json, result_json, provider_metadata,
  requested_by_channel_identity_id, responded_by_channel_identity_id,
  assistant_message_id, tool_result_message_id, prompt_message_id,
  persist_branch_id, persist_turn_id, prompt_external_message_id,
  source_platform, reply_target, conversation_type, expires_at,
  created_at, responded_at, canceled_at, updated_at
)
SELECT
  id, bot_id, session_id, route_id, channel_identity_id, tool_call_id, tool_name,
  short_id, status, input_json, ui_payload_json, result_json, provider_metadata,
  requested_by_channel_identity_id, responded_by_channel_identity_id,
  assistant_message_id, tool_result_message_id, prompt_message_id,
  NULL, NULL,
  prompt_external_message_id, source_platform, reply_target, conversation_type,
  expires_at, created_at, responded_at, canceled_at, updated_at
FROM user_input_requests;

DROP TABLE user_input_requests;
ALTER TABLE user_input_requests_0023_new RENAME TO user_input_requests;

CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_unique
  ON user_input_requests(session_id, tool_call_id);
CREATE INDEX IF NOT EXISTS idx_user_input_bot_status_created
  ON user_input_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_session_status_created
  ON user_input_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_prompt_external
  ON user_input_requests(prompt_external_message_id)
  WHERE prompt_external_message_id != '';

COMMIT;

PRAGMA foreign_keys = ON;
