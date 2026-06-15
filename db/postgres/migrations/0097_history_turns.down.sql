-- 0097_history_turns
-- Remove first-class history turns.

DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_turns_assistant;
DROP INDEX IF EXISTS idx_bot_history_turns_request;
DROP INDEX IF EXISTS idx_bot_history_turns_session_branch;
DROP INDEX IF EXISTS idx_bot_history_turns_branch_seq;

ALTER TABLE bot_session_branches
  DROP CONSTRAINT IF EXISTS fk_bot_session_branches_fork_from_turn;
ALTER TABLE bot_history_messages
  DROP CONSTRAINT IF EXISTS fk_bot_history_messages_turn;

ALTER TABLE bot_history_messages
  DROP COLUMN IF EXISTS turn_message_seq,
  DROP COLUMN IF EXISTS turn_id;

ALTER TABLE bot_session_branches
  DROP COLUMN IF EXISTS fork_from_turn_seq,
  DROP COLUMN IF EXISTS fork_from_turn_id;

DROP TABLE IF EXISTS bot_history_turns;
