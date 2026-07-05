-- 0029_message_turn_read_model
-- Materialize turn order and visibility on messages for hot pagination reads.

PRAGMA foreign_keys = OFF;

BEGIN;

DROP VIEW IF EXISTS bot_visible_history_messages;

DROP INDEX IF EXISTS idx_bot_history_messages_visible_session_source_order;
DROP INDEX IF EXISTS idx_bot_history_messages_visible_session_order;
DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_messages_session_role_created;
DROP INDEX IF EXISTS idx_bot_history_messages_session_reply;
DROP INDEX IF EXISTS idx_bot_history_messages_session_source;
DROP INDEX IF EXISTS idx_bot_history_messages_session;
DROP INDEX IF EXISTS idx_bot_history_messages_compact;
DROP INDEX IF EXISTS idx_bot_history_messages_bot_created;

CREATE TABLE bot_history_messages_new (
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
  session_mode TEXT NOT NULL DEFAULT 'chat' CHECK (session_mode IN ('chat', 'discuss', 'heartbeat', 'schedule', 'subagent')),
  runtime_type TEXT NOT NULL DEFAULT 'model' CHECK (runtime_type IN ('model', 'acp_agent')),
  model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  compact_id TEXT,
  event_id TEXT REFERENCES bot_session_events(id) ON DELETE SET NULL,
  display_text TEXT,
  turn_id TEXT,
  turn_position INTEGER,
  turn_message_seq INTEGER,
  turn_visible INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bot_history_messages_new (
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
  session_mode,
  runtime_type,
  model_id,
  compact_id,
  event_id,
  display_text,
  turn_id,
  turn_position,
  turn_message_seq,
  turn_visible,
  created_at
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id,
  m.source_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.model_id,
  m.compact_id,
  m.event_id,
  m.display_text,
  m.turn_id,
  t.position,
  m.turn_message_seq,
  CASE WHEN t.id IS NOT NULL AND t.superseded_at IS NULL THEN 1 ELSE 0 END,
  m.created_at
FROM bot_history_messages m
LEFT JOIN bot_history_turns t ON t.id = m.turn_id;

DROP TABLE bot_history_messages;
ALTER TABLE bot_history_messages_new RENAME TO bot_history_messages;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_compact ON bot_history_messages(compact_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session
  ON bot_history_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_role_created
  ON bot_history_messages(session_id, role, created_at, id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn
  ON bot_history_messages(turn_id, turn_message_seq, created_at, id)
  WHERE turn_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_order
  ON bot_history_messages(session_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = 1
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_source_order
  ON bot_history_messages(session_id, source_message_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = 1
    AND source_message_id IS NOT NULL
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_source
  ON bot_history_messages(session_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_reply
  ON bot_history_messages(session_id, source_reply_to_message_id);

CREATE VIEW IF NOT EXISTS bot_visible_history_messages AS
SELECT
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id,
  m.source_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.compact_id,
  m.session_mode,
  m.runtime_type,
  m.model_id,
  m.event_id,
  m.display_text,
  m.created_at
FROM bot_history_messages m
WHERE m.turn_visible = 1;

COMMIT;

PRAGMA foreign_keys = ON;
