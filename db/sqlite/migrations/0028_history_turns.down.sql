-- 0028_history_turns (down)
-- Remove linear history turns.

DROP INDEX IF EXISTS idx_bot_history_turns_assistant_message;
DROP INDEX IF EXISTS idx_bot_history_turns_request_message;
DROP INDEX IF EXISTS idx_bot_history_turns_session_active;
DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_messages_session_role_created;
DROP VIEW IF EXISTS bot_visible_history_messages;
DROP TABLE IF EXISTS bot_history_turns;
ALTER TABLE bot_history_messages DROP COLUMN turn_message_seq;
ALTER TABLE bot_history_messages DROP COLUMN turn_id;
