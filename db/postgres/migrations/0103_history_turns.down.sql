-- 0103_history_turns (down)
-- Remove linear history turns, hot message turn read model, and session turn position allocator.

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

DROP VIEW IF EXISTS bot_visible_history_messages;

DROP INDEX IF EXISTS idx_bot_history_messages_visible_session_source_order;
DROP INDEX IF EXISTS idx_bot_history_messages_visible_session_order;

DROP INDEX IF EXISTS idx_bot_history_turns_assistant_message;
DROP INDEX IF EXISTS idx_bot_history_turns_request_message;
DROP INDEX IF EXISTS idx_bot_history_turns_session_active;
DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_messages_session_role_created;
DROP TABLE IF EXISTS bot_history_turns;
ALTER TABLE bot_history_messages
  DROP COLUMN IF EXISTS turn_visible,
  DROP COLUMN IF EXISTS turn_position,
  DROP COLUMN IF EXISTS turn_message_seq,
  DROP COLUMN IF EXISTS turn_id;
