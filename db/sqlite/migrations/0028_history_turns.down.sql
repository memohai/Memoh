-- 0028_history_turns (down)
-- Remove linear history turns, hot message turn read model, and session turn position allocator.

-- Keep full rollback atomic with the older ACP session-type guard. If acp_agent
-- sessions exist, migration 0007 down will fail, so fail before changing
-- bot_sessions here.
BEGIN;

CREATE TEMP TABLE IF NOT EXISTS _memoh_acp_session_type_down_guard (
  ok INTEGER NOT NULL CHECK (ok = 1)
);

INSERT INTO _memoh_acp_session_type_down_guard(ok)
SELECT 0 WHERE EXISTS (SELECT 1 FROM bot_sessions WHERE type = 'acp_agent');

DROP TABLE _memoh_acp_session_type_down_guard;

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
ALTER TABLE bot_sessions
  DROP COLUMN next_turn_position;
ALTER TABLE bot_history_messages DROP COLUMN turn_visible;
ALTER TABLE bot_history_messages DROP COLUMN turn_position;
ALTER TABLE bot_history_messages DROP COLUMN turn_message_seq;
ALTER TABLE bot_history_messages DROP COLUMN turn_id;

COMMIT;
