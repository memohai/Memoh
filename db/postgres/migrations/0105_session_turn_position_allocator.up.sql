-- 0105_session_turn_position_allocator
-- Add a per-session turn position allocator for concurrent append writes.

ALTER TABLE bot_sessions
  ADD COLUMN IF NOT EXISTS next_turn_position BIGINT NOT NULL DEFAULT 1;

UPDATE bot_sessions s
SET next_turn_position = GREATEST(s.next_turn_position, turns.next_position)
FROM (
  SELECT session_id, MAX(position) + 1 AS next_position
  FROM bot_history_turns
  GROUP BY session_id
) turns
WHERE s.id = turns.session_id;
