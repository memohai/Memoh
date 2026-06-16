-- 0099_pending_branch_turn_context
-- Remove deferred continuation branch/turn context columns.

ALTER TABLE user_input_requests
  DROP COLUMN IF EXISTS persist_turn_id,
  DROP COLUMN IF EXISTS persist_branch_id;

ALTER TABLE tool_approval_requests
  DROP COLUMN IF EXISTS persist_turn_id,
  DROP COLUMN IF EXISTS persist_branch_id;
