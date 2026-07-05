-- 0029_message_turn_read_model
-- Materialize turn order and visibility on messages for hot pagination reads.

ALTER TABLE bot_history_messages
  ADD COLUMN turn_position INTEGER;

ALTER TABLE bot_history_messages
  ADD COLUMN turn_visible INTEGER NOT NULL DEFAULT 0;

UPDATE bot_history_messages
SET turn_position = (
      SELECT t.position
      FROM bot_history_turns t
      WHERE t.id = bot_history_messages.turn_id
    ),
    turn_visible = COALESCE((
      SELECT CASE WHEN t.superseded_at IS NULL THEN 1 ELSE 0 END
      FROM bot_history_turns t
      WHERE t.id = bot_history_messages.turn_id
    ), 0)
WHERE turn_id IS NOT NULL;

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

DROP VIEW IF EXISTS bot_visible_history_messages;

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
