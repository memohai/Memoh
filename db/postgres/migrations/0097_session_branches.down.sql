-- 0097_session_branches
-- Remove in-session branch paths and history turns.

DROP VIEW IF EXISTS bot_branch_visible_messages;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_turns_assistant;
DROP INDEX IF EXISTS idx_bot_history_turns_request;
DROP INDEX IF EXISTS idx_bot_history_turns_session_branch;
DROP INDEX IF EXISTS idx_bot_history_turns_branch_seq;
DROP INDEX IF EXISTS idx_bot_history_messages_branch;
DROP INDEX IF EXISTS idx_bot_history_messages_branch_seq;
DROP INDEX IF EXISTS idx_bot_session_branches_parent;
DROP INDEX IF EXISTS idx_bot_session_branches_session;
DROP INDEX IF EXISTS idx_bot_session_branches_root;

ALTER TABLE bot_session_branches
  DROP CONSTRAINT IF EXISTS fk_bot_session_branches_fork_from_turn;
ALTER TABLE bot_history_messages
  DROP CONSTRAINT IF EXISTS fk_bot_history_messages_turn;

ALTER TABLE bot_history_messages
  DROP COLUMN IF EXISTS turn_message_seq,
  DROP COLUMN IF EXISTS turn_id,
  DROP COLUMN IF EXISTS branch_seq,
  DROP COLUMN IF EXISTS branch_id;

ALTER TABLE bot_sessions
  DROP COLUMN IF EXISTS active_branch_id;

DROP TABLE IF EXISTS bot_history_turns;
DROP TABLE IF EXISTS bot_session_branches;
