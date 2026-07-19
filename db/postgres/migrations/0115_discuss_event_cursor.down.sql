-- 0115_discuss_event_cursor
-- Remove the strict event cursor while retaining the rollback source-time cursor.

ALTER TABLE bot_session_discuss_cursors
  DROP COLUMN IF EXISTS consumed_event_cursor;
