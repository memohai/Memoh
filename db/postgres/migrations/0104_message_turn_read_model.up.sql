-- 0104_message_turn_read_model
-- Materialize turn order and visibility on messages for hot pagination reads.

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS turn_position BIGINT,
  ADD COLUMN IF NOT EXISTS turn_visible BOOLEAN NOT NULL DEFAULT false;

UPDATE bot_history_messages m
SET turn_position = t.position,
    turn_visible = (t.superseded_at IS NULL)
FROM bot_history_turns t
WHERE m.turn_id = t.id;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_order
  ON bot_history_messages(session_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = true
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_source_order
  ON bot_history_messages(session_id, source_message_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = true
    AND source_message_id IS NOT NULL
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;

CREATE OR REPLACE VIEW bot_visible_history_messages AS
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
WHERE m.turn_visible = true;
