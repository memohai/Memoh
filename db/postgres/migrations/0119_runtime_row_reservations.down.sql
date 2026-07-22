-- 0119_runtime_row_reservations
-- Remove the runtime row coordinate uniqueness guard.
DROP INDEX IF EXISTS idx_bot_history_messages_session_position_seq_unique;
