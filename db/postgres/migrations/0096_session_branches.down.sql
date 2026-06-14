-- 0096_session_branches
-- Remove in-session branch paths.

DROP INDEX IF EXISTS idx_bot_history_messages_branch;
DROP INDEX IF EXISTS idx_bot_history_messages_branch_seq;
DROP INDEX IF EXISTS idx_bot_session_branches_parent;
DROP INDEX IF EXISTS idx_bot_session_branches_session;
DROP INDEX IF EXISTS idx_bot_session_branches_root;

ALTER TABLE bot_history_messages
  DROP COLUMN IF EXISTS branch_seq,
  DROP COLUMN IF EXISTS branch_id;

ALTER TABLE bot_sessions
  DROP COLUMN IF EXISTS active_branch_id;

DROP TABLE IF EXISTS bot_session_branches;
