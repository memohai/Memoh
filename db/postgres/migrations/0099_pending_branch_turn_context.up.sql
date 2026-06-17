-- 0099_pending_branch_turn_context
-- Persist branch/turn context for deferred tool approval and ask_user continuations.

ALTER TABLE tool_approval_requests
  ADD COLUMN IF NOT EXISTS persist_branch_id UUID REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS persist_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL;

ALTER TABLE user_input_requests
  ADD COLUMN IF NOT EXISTS persist_branch_id UUID REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS persist_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL;

ALTER TABLE tool_approval_requests
  DROP CONSTRAINT IF EXISTS tool_approval_tool_call_unique;

ALTER TABLE user_input_requests
  DROP CONSTRAINT IF EXISTS user_input_tool_call_unique;

CREATE UNIQUE INDEX IF NOT EXISTS tool_approval_tool_call_legacy_unique
  ON tool_approval_requests(session_id, tool_call_id)
  WHERE persist_turn_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS tool_approval_tool_call_turn_unique
  ON tool_approval_requests(session_id, tool_call_id, persist_turn_id)
  WHERE persist_turn_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_legacy_unique
  ON user_input_requests(session_id, tool_call_id)
  WHERE persist_turn_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_turn_unique
  ON user_input_requests(session_id, tool_call_id, persist_turn_id)
  WHERE persist_turn_id IS NOT NULL;
