-- 0099_pending_branch_turn_context
-- Remove deferred continuation branch/turn context columns.

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
