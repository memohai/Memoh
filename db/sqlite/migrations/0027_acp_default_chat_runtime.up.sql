-- 0027_acp_default_chat_runtime
-- Add bot default chat runtime fields and split session mode from runtime type.

PRAGMA foreign_keys = OFF;

BEGIN;

ALTER TABLE bots ADD COLUMN chat_runtime TEXT NOT NULL DEFAULT 'model' CHECK (chat_runtime IN ('model', 'acp_agent'));
ALTER TABLE bots ADD COLUMN chat_acp_agent_id TEXT;
ALTER TABLE bots ADD COLUMN chat_acp_project_path TEXT NOT NULL DEFAULT '/data';
ALTER TABLE bots ADD COLUMN chat_acp_project_mode TEXT NOT NULL DEFAULT 'project' CHECK (chat_acp_project_mode IN ('project', 'none'));

ALTER TABLE bot_sessions ADD COLUMN session_mode TEXT NOT NULL DEFAULT 'chat' CHECK (session_mode IN ('chat', 'discuss', 'heartbeat', 'schedule', 'subagent'));
ALTER TABLE bot_sessions ADD COLUMN runtime_type TEXT NOT NULL DEFAULT 'model' CHECK (runtime_type IN ('model', 'acp_agent'));
ALTER TABLE bot_sessions ADD COLUMN runtime_metadata TEXT NOT NULL DEFAULT '{}';

UPDATE bot_sessions
SET session_mode = CASE
      WHEN type = 'acp_agent' THEN 'chat'
      ELSE type
    END,
    runtime_type = CASE
      WHEN type = 'acp_agent' THEN 'acp_agent'
      ELSE 'model'
    END,
    runtime_metadata = CASE
      WHEN type = 'acp_agent' THEN json_patch(
        json_object(
          'project_path', COALESCE(NULLIF(json_extract(metadata, '$.project_path'), ''), '/data'),
          'acp_project_mode', COALESCE(NULLIF(json_extract(metadata, '$.acp_project_mode'), ''), 'project')
        ),
        -- json_patch drops keys whose patch value is NULL (RFC 7386 merge),
        -- mirroring Postgres jsonb_strip_nulls. Older ACP rows may not have
        -- runtime_owner_account_id in metadata, so fall back to the session
        -- creator when available.
        json_object(
          'acp_agent_id', json_extract(metadata, '$.acp_agent_id'),
          'runtime_owner_account_id', COALESCE(NULLIF(json_extract(metadata, '$.runtime_owner_account_id'), ''), created_by_user_id)
        )
      )
      ELSE '{}'
    END
WHERE session_mode = 'chat'
  AND runtime_type = 'model'
  AND runtime_metadata = '{}';

CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_mode_runtime_active_updated
  ON bot_sessions(bot_id, session_mode, runtime_type, updated_at DESC, id DESC)
  WHERE deleted_at IS NULL;

CREATE TABLE bot_history_messages_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  turn_message_seq INTEGER,
  sender_channel_identity_id TEXT REFERENCES channel_identities(id),
  sender_account_user_id TEXT REFERENCES users(id),
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
  content TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat' CHECK (session_mode IN ('chat', 'discuss', 'heartbeat', 'schedule', 'subagent')),
  runtime_type TEXT NOT NULL DEFAULT 'model' CHECK (runtime_type IN ('model', 'acp_agent')),
  model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  compact_id TEXT,
  event_id TEXT REFERENCES bot_session_events(id) ON DELETE SET NULL,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bot_history_messages_new (
  id, bot_id, session_id, turn_id, turn_message_seq,
  sender_channel_identity_id, sender_account_user_id, source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, session_mode, runtime_type, model_id, compact_id, event_id, display_text, created_at
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id,
  m.source_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  COALESCE(CASE WHEN s.type = 'subagent' THEN ps.session_mode ELSE s.session_mode END, 'chat') AS session_mode,
  COALESCE(CASE WHEN s.type = 'subagent' THEN ps.runtime_type ELSE s.runtime_type END, 'model') AS runtime_type,
  m.model_id,
  m.compact_id,
  m.event_id,
  m.display_text,
  m.created_at
FROM bot_history_messages AS m
LEFT JOIN bot_sessions AS s ON s.id = m.session_id
LEFT JOIN bot_sessions AS ps ON ps.id = s.parent_session_id;

DROP TABLE bot_history_messages;
ALTER TABLE bot_history_messages_new RENAME TO bot_history_messages;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_compact ON bot_history_messages(compact_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session
  ON bot_history_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn
  ON bot_history_messages(turn_id, turn_message_seq, created_at)
  WHERE turn_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_source
  ON bot_history_messages(session_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_reply
  ON bot_history_messages(session_id, source_reply_to_message_id);

CREATE TABLE IF NOT EXISTS bot_session_discuss_cursors (
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  scope_key TEXT NOT NULL DEFAULT 'default',
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  source TEXT NOT NULL DEFAULT '',
  consumed_cursor INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (session_id, scope_key)
);

CREATE INDEX IF NOT EXISTS idx_bot_session_discuss_cursors_route
  ON bot_session_discuss_cursors(route_id)
  WHERE route_id IS NOT NULL;

COMMIT;

PRAGMA foreign_keys = ON;
