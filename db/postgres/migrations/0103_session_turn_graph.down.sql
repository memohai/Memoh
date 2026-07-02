-- 0103_session_turn_graph (down)
-- Downgrading removes the turn graph model. Tool approval and user input rows
-- that were distinct only by persist_turn_id are folded back to the old
-- (session_id, tool_call_id) uniqueness shape, keeping the newest row.

DROP INDEX IF EXISTS idx_user_input_persist_turn;
DROP INDEX IF EXISTS idx_tool_approval_persist_turn;
DROP INDEX IF EXISTS user_input_tool_call_turn_unique;
DROP INDEX IF EXISTS user_input_tool_call_legacy_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_turn_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_legacy_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_turns_assistant;
DROP INDEX IF EXISTS idx_bot_history_turns_request;
DROP INDEX IF EXISTS idx_bot_history_turns_parent;
DROP INDEX IF EXISTS idx_bot_history_turns_owner_session;
DROP INDEX IF EXISTS idx_bot_history_turns_bot_created;
DROP INDEX IF EXISTS idx_bot_session_turn_heads_head;
DROP INDEX IF EXISTS idx_bot_session_turn_heads_bot;
DROP INDEX IF EXISTS idx_bot_sessions_forked_from_turn;
DROP INDEX IF EXISTS idx_bot_sessions_forked_from_session;
DROP INDEX IF EXISTS idx_bot_sessions_default_head_turn;

DROP TRIGGER IF EXISTS user_input_persist_turn_owner_guard ON user_input_requests;
DROP TRIGGER IF EXISTS tool_approval_persist_turn_owner_guard ON tool_approval_requests;
DROP FUNCTION IF EXISTS enforce_request_persist_turn_owner();

ALTER TABLE user_input_requests DROP CONSTRAINT IF EXISTS fk_user_input_persist_turn;
ALTER TABLE tool_approval_requests DROP CONSTRAINT IF EXISTS fk_tool_approval_persist_turn;
ALTER TABLE user_input_requests DROP CONSTRAINT IF EXISTS fk_user_input_session_bot;
ALTER TABLE tool_approval_requests DROP CONSTRAINT IF EXISTS fk_tool_approval_session_bot;
ALTER TABLE bot_history_messages DROP CONSTRAINT IF EXISTS fk_bot_history_messages_turn;

WITH ranked_tool_approvals AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY session_id, tool_call_id
      ORDER BY created_at DESC, id DESC
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
      ORDER BY created_at DESC, id DESC
    ) AS row_num
  FROM user_input_requests
)
DELETE FROM user_input_requests request
USING ranked_user_inputs ranked
WHERE request.id = ranked.id
  AND ranked.row_num > 1;

ALTER TABLE user_input_requests DROP COLUMN IF EXISTS persist_turn_id;
ALTER TABLE tool_approval_requests DROP COLUMN IF EXISTS persist_turn_id;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tool_approval_tool_call_unique'
  ) THEN
    ALTER TABLE tool_approval_requests
      ADD CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'user_input_tool_call_unique'
  ) THEN
    ALTER TABLE user_input_requests
      ADD CONSTRAINT user_input_tool_call_unique UNIQUE (session_id, tool_call_id);
  END IF;
END $$;

ALTER TABLE bot_sessions DROP CONSTRAINT IF EXISTS fk_bot_sessions_default_head_turn;
ALTER TABLE bot_sessions DROP CONSTRAINT IF EXISTS fk_bot_sessions_forked_from_turn;
ALTER TABLE bot_session_turn_heads DROP CONSTRAINT IF EXISTS fk_bot_session_turn_heads_session_bot;
ALTER TABLE bot_session_turn_heads DROP CONSTRAINT IF EXISTS fk_bot_session_turn_heads_turn_bot;
ALTER TABLE bot_history_turns DROP CONSTRAINT IF EXISTS fk_bot_history_turns_request_message;
ALTER TABLE bot_history_turns DROP CONSTRAINT IF EXISTS fk_bot_history_turns_final_assistant_message;

ALTER TABLE bot_history_messages DROP COLUMN IF EXISTS turn_message_seq;
ALTER TABLE bot_history_messages DROP COLUMN IF EXISTS turn_id;

ALTER TABLE bot_sessions DROP COLUMN IF EXISTS forked_from_turn_id;
ALTER TABLE bot_sessions DROP COLUMN IF EXISTS forked_from_session_id;
ALTER TABLE bot_sessions DROP COLUMN IF EXISTS default_head_turn_id;

DROP TABLE IF EXISTS bot_session_turn_heads;
DROP TABLE IF EXISTS bot_history_turns;
DROP INDEX IF EXISTS idx_bot_sessions_id_bot_unique;
