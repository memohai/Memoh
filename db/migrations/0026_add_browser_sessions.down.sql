-- 0021_add_browser_sessions (rollback)
-- Remove browser_sessions table and indexes

DROP INDEX IF EXISTS idx_browser_sessions_expires_at;
DROP INDEX IF EXISTS idx_browser_sessions_bot_status;
DROP INDEX IF EXISTS idx_browser_sessions_session_id;
DROP TABLE IF EXISTS browser_sessions;
