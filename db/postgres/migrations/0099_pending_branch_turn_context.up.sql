-- 0099_pending_branch_turn_context
-- Persist branch/turn context for deferred tool approval and ask_user continuations.

ALTER TABLE tool_approval_requests
  ADD COLUMN IF NOT EXISTS persist_branch_id UUID REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS persist_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL;

ALTER TABLE user_input_requests
  ADD COLUMN IF NOT EXISTS persist_branch_id UUID REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS persist_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL;
