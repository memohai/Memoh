-- 0030_session_turn_position_allocator
-- Add a per-session turn position allocator for concurrent append writes.

ALTER TABLE bot_sessions
  ADD COLUMN next_turn_position INTEGER NOT NULL DEFAULT 1;

UPDATE bot_sessions
SET next_turn_position = MAX(
  next_turn_position,
  COALESCE((
    SELECT MAX(position) + 1
    FROM bot_history_turns
    WHERE bot_history_turns.session_id = bot_sessions.id
  ), 1)
);
