-- 0104_message_turn_read_model
-- Remove materialized message turn read-model columns.

DROP VIEW IF EXISTS bot_visible_history_messages;

DROP INDEX IF EXISTS idx_bot_history_messages_visible_session_source_order;
DROP INDEX IF EXISTS idx_bot_history_messages_visible_session_order;

ALTER TABLE bot_history_messages
  DROP COLUMN IF EXISTS turn_visible,
  DROP COLUMN IF EXISTS turn_position;

CREATE OR REPLACE VIEW bot_visible_history_messages AS
SELECT
  t.id AS turn_id,
  t.position AS turn_position,
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
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE t.superseded_at IS NULL;
