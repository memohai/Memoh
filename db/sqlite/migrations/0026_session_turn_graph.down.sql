-- 0026_session_turn_graph (down)
-- Remove immutable turn chains and session-level fork pointers.
-- Downgrading removes the turn graph model. Tool approval and user input rows
-- that were distinct only by persist_turn_id are folded back to the old
-- (session_id, tool_call_id) uniqueness shape, keeping the newest row.

PRAGMA foreign_keys = OFF;

BEGIN;

DROP INDEX IF EXISTS idx_user_input_persist_turn;
DROP INDEX IF EXISTS idx_tool_approval_persist_turn;
DROP INDEX IF EXISTS user_input_tool_call_turn_unique;
DROP INDEX IF EXISTS user_input_tool_call_legacy_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_turn_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_legacy_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_turns_assistant;
DROP INDEX IF EXISTS idx_bot_history_turns_request;
DROP INDEX IF EXISTS idx_bot_history_turns_parent;
DROP INDEX IF EXISTS idx_bot_history_turns_owner_session;
DROP INDEX IF EXISTS idx_bot_history_turns_bot_created;
DROP INDEX IF EXISTS idx_bot_history_turns_id_bot_unique;
DROP INDEX IF EXISTS idx_bot_session_turn_heads_head;
DROP INDEX IF EXISTS idx_bot_session_turn_heads_bot;
DROP INDEX IF EXISTS idx_bot_sessions_id_bot_unique;
DROP INDEX IF EXISTS idx_bot_sessions_forked_from_turn;
DROP INDEX IF EXISTS idx_bot_sessions_forked_from_session;
DROP INDEX IF EXISTS idx_bot_sessions_default_head_turn;

DROP TRIGGER IF EXISTS user_input_persist_turn_owner_update;
DROP TRIGGER IF EXISTS user_input_persist_turn_owner_insert;
DROP TRIGGER IF EXISTS tool_approval_persist_turn_owner_update;
DROP TRIGGER IF EXISTS tool_approval_persist_turn_owner_insert;

CREATE TABLE bot_sessions_0026_down (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_type TEXT,
  type TEXT NOT NULL DEFAULT 'chat' CHECK (type IN ('chat', 'heartbeat', 'schedule', 'subagent', 'discuss', 'acp_agent')),
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  parent_session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);

INSERT INTO bot_sessions_0026_down (
  id,
  bot_id,
  route_id,
  channel_type,
  type,
  title,
  metadata,
  parent_session_id,
  created_by_user_id,
  created_at,
  updated_at,
  deleted_at
)
SELECT
  id,
  bot_id,
  route_id,
  channel_type,
  type,
  title,
  metadata,
  parent_session_id,
  created_by_user_id,
  created_at,
  updated_at,
  deleted_at
FROM bot_sessions;

DROP TABLE bot_sessions;
ALTER TABLE bot_sessions_0026_down RENAME TO bot_sessions;

CREATE TABLE bot_history_messages_0026_down (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  sender_channel_identity_id TEXT REFERENCES channel_identities(id),
  sender_account_user_id TEXT REFERENCES users(id),
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
  content TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  compact_id TEXT,
  event_id TEXT REFERENCES bot_session_events(id) ON DELETE SET NULL,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bot_history_messages_0026_down (
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  model_id,
  compact_id,
  event_id,
  display_text,
  created_at
)
SELECT
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  model_id,
  compact_id,
  event_id,
  display_text,
  created_at
FROM bot_history_messages;

DROP TABLE bot_history_messages;
ALTER TABLE bot_history_messages_0026_down RENAME TO bot_history_messages;

DROP TABLE IF EXISTS bot_session_turn_heads;
DROP TABLE IF EXISTS bot_history_turns;

CREATE TABLE tool_approval_requests_0026_down (
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

INSERT INTO tool_approval_requests_0026_down (
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  operation,
  tool_input,
  short_id,
  status,
  decision_reason,
  requested_by_channel_identity_id,
  decided_by_channel_identity_id,
  requested_message_id,
  prompt_message_id,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  created_at,
  decided_at
)
WITH ranked_tool_approvals AS (
  SELECT
    *,
    ROW_NUMBER() OVER (
      PARTITION BY session_id, tool_call_id
      ORDER BY created_at DESC, id DESC
    ) AS row_num
  FROM tool_approval_requests
)
SELECT
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  operation,
  tool_input,
  short_id,
  status,
  decision_reason,
  requested_by_channel_identity_id,
  decided_by_channel_identity_id,
  requested_message_id,
  prompt_message_id,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  created_at,
  decided_at
FROM ranked_tool_approvals
WHERE row_num = 1;

DROP TABLE tool_approval_requests;
ALTER TABLE tool_approval_requests_0026_down RENAME TO tool_approval_requests;

CREATE TABLE user_input_requests_0026_down (
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

INSERT INTO user_input_requests_0026_down (
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  short_id,
  status,
  input_json,
  ui_payload_json,
  result_json,
  provider_metadata,
  requested_by_channel_identity_id,
  responded_by_channel_identity_id,
  assistant_message_id,
  tool_result_message_id,
  prompt_message_id,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  expires_at,
  created_at,
  responded_at,
  canceled_at,
  updated_at
)
WITH ranked_user_inputs AS (
  SELECT
    *,
    ROW_NUMBER() OVER (
      PARTITION BY session_id, tool_call_id
      ORDER BY created_at DESC, id DESC
    ) AS row_num
  FROM user_input_requests
)
SELECT
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  short_id,
  status,
  input_json,
  ui_payload_json,
  result_json,
  provider_metadata,
  requested_by_channel_identity_id,
  responded_by_channel_identity_id,
  assistant_message_id,
  tool_result_message_id,
  prompt_message_id,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  expires_at,
  created_at,
  responded_at,
  canceled_at,
  updated_at
FROM ranked_user_inputs
WHERE row_num = 1;

DROP TABLE user_input_requests;
ALTER TABLE user_input_requests_0026_down RENAME TO user_input_requests;

CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_id ON bot_sessions(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_route_id ON bot_sessions(route_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active ON bot_sessions(bot_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_parent ON bot_sessions(parent_session_id) WHERE parent_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_created_by_user_id ON bot_sessions(created_by_user_id) WHERE created_by_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_created_by ON bot_sessions(bot_id, created_by_user_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active_updated ON bot_sessions(bot_id, updated_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_compact ON bot_history_messages(compact_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session ON bot_history_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_source ON bot_history_messages(session_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_reply ON bot_history_messages(session_id, source_reply_to_message_id);

CREATE INDEX IF NOT EXISTS idx_tool_approval_bot_status_created ON tool_approval_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_session_status_created ON tool_approval_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_prompt_external ON tool_approval_requests(prompt_external_message_id) WHERE prompt_external_message_id != '';

CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_unique ON user_input_requests(session_id, tool_call_id);
CREATE INDEX IF NOT EXISTS idx_user_input_bot_status_created ON user_input_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_session_status_created ON user_input_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_prompt_external ON user_input_requests(prompt_external_message_id) WHERE prompt_external_message_id != '';

COMMIT;

PRAGMA foreign_keys = ON;
