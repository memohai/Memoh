-- 0119_runtime_row_reservations
-- Enforce one durable row identity at each reserved session order coordinate.
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_session_position_seq_unique
  ON bot_history_messages(session_id, turn_position, turn_message_seq)
  WHERE session_id IS NOT NULL AND turn_position IS NOT NULL AND turn_message_seq IS NOT NULL;
