-- 0098_session_branches
-- Remove in-session branch paths and history turns.

DROP VIEW IF EXISTS bot_branch_visible_messages;

-- Downgrading loses per-turn identity. Keep one deterministic legacy row per
-- (session_id, tool_call_id): existing legacy rows first, then newest turn row.
WITH ranked_tool_approvals AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY session_id, tool_call_id
      ORDER BY
        CASE WHEN persist_turn_id IS NULL THEN 0 ELSE 1 END,
        created_at DESC,
        id DESC
    ) AS row_num
  FROM tool_approval_requests
)
DELETE FROM tool_approval_requests request
USING ranked_tool_approvals ranked
WHERE request.id = ranked.id
  AND ranked.row_num > 1;

WITH ranked_user_inputs AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY session_id, tool_call_id
      ORDER BY
        CASE WHEN persist_turn_id IS NULL THEN 0 ELSE 1 END,
        created_at DESC,
        id DESC
    ) AS row_num
  FROM user_input_requests
)
DELETE FROM user_input_requests request
USING ranked_user_inputs ranked
WHERE request.id = ranked.id
  AND ranked.row_num > 1;

DROP INDEX IF EXISTS user_input_tool_call_turn_unique;
DROP INDEX IF EXISTS user_input_tool_call_legacy_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_turn_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_legacy_unique;

ALTER TABLE tool_approval_requests
  ADD CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id);

ALTER TABLE user_input_requests
  ADD CONSTRAINT user_input_tool_call_unique UNIQUE (session_id, tool_call_id);

ALTER TABLE user_input_requests
  DROP COLUMN IF EXISTS persist_turn_id,
  DROP COLUMN IF EXISTS persist_branch_id;

ALTER TABLE tool_approval_requests
  DROP COLUMN IF EXISTS persist_turn_id,
  DROP COLUMN IF EXISTS persist_branch_id;

DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_turns_assistant;
DROP INDEX IF EXISTS idx_bot_history_turns_request;
DROP INDEX IF EXISTS idx_bot_history_turns_session_branch;
DROP INDEX IF EXISTS idx_bot_history_turns_branch_seq;
DROP INDEX IF EXISTS idx_bot_history_messages_branch_role_seq;
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
