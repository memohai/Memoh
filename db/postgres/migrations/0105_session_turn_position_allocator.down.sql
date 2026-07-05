-- 0105_session_turn_position_allocator
-- Remove the per-session turn position allocator.

-- Keep full rollback atomic with the older ACP session-type guard. If acp_agent
-- sessions exist, migration 0082 down will fail, so fail before changing
-- bot_sessions here.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM bot_sessions WHERE type = 'acp_agent') THEN
    RAISE EXCEPTION 'cannot remove session turn position allocator while acp_agent sessions exist';
  END IF;
END $$;

ALTER TABLE bot_sessions
  DROP COLUMN IF EXISTS next_turn_position;
