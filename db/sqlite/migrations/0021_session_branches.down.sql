-- 0021_session_branches
-- Remove in-session branch paths.

CREATE TEMP TABLE IF NOT EXISTS _memoh_session_branches_down_guard (
  ok INTEGER NOT NULL CHECK (ok = 1)
);

INSERT INTO _memoh_session_branches_down_guard(ok)
SELECT 0 WHERE EXISTS (SELECT 1 FROM bot_sessions WHERE type = 'acp_agent');

DROP TABLE _memoh_session_branches_down_guard;

BEGIN;

DROP INDEX IF EXISTS idx_bot_history_messages_branch;
DROP INDEX IF EXISTS idx_bot_history_messages_branch_seq;
DROP INDEX IF EXISTS idx_bot_session_branches_parent;
DROP INDEX IF EXISTS idx_bot_session_branches_session;
DROP INDEX IF EXISTS idx_bot_session_branches_root;

ALTER TABLE bot_history_messages DROP COLUMN branch_seq;
ALTER TABLE bot_history_messages DROP COLUMN branch_id;
ALTER TABLE bot_sessions DROP COLUMN active_branch_id;

DROP TABLE IF EXISTS bot_session_branches;

COMMIT;
