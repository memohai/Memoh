-- 0010_session_created_by_user
-- Track the user that created a bot session so shared chat members only see their own sessions.

ALTER TABLE bot_sessions
  ADD COLUMN created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_bot_sessions_created_by_user_id
  ON bot_sessions(created_by_user_id)
  WHERE created_by_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_created_by
  ON bot_sessions(bot_id, created_by_user_id, deleted_at);
