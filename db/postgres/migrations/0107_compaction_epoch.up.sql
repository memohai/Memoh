-- 0107_compaction_epoch
-- Fence session compaction artifacts when their source history is rewritten.

ALTER TABLE bot_sessions
  ADD COLUMN IF NOT EXISTS compaction_epoch BIGINT NOT NULL DEFAULT 0;

ALTER TABLE bot_history_message_compacts
  ADD COLUMN IF NOT EXISTS compaction_epoch BIGINT NOT NULL DEFAULT 0;

ALTER TABLE bot_history_message_compacts
  DROP CONSTRAINT IF EXISTS bot_history_message_compacts_session_id_fkey;
ALTER TABLE bot_history_message_compacts
  ADD CONSTRAINT bot_history_message_compacts_session_id_fkey
  FOREIGN KEY (session_id) REFERENCES bot_sessions(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_compacts_owner_epoch
  ON bot_history_message_compacts(bot_id, session_id, compaction_epoch, started_at DESC);

UPDATE bot_sessions session
SET compaction_epoch = session.compaction_epoch + 1
WHERE EXISTS (
  SELECT 1
  FROM bot_history_messages message
  JOIN bot_history_message_compacts compact ON compact.id = message.compact_id
  WHERE message.session_id = session.id
    AND compact.bot_id = session.bot_id
    AND compact.session_id = session.id
    AND compact.compaction_epoch = session.compaction_epoch
    AND (
      message.turn_visible = false
      OR message.turn_id IS NULL
      OR message.turn_position IS NULL
      OR message.turn_message_seq IS NULL
      OR message.turn_superseded_at IS NOT NULL
    )
);
