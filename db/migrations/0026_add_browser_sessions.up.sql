-- 0021_add_browser_sessions
-- Add browser_sessions table for per-bot browser session lifecycle

CREATE TABLE IF NOT EXISTS browser_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'remote',
  remote_session_id TEXT NOT NULL,
  worker_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed', 'expired', 'error')),
  current_url TEXT NOT NULL DEFAULT '',
  context_dir TEXT NOT NULL DEFAULT '',
  idle_ttl_seconds INTEGER NOT NULL DEFAULT 600,
  action_count INTEGER NOT NULL DEFAULT 0,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_browser_sessions_session_id ON browser_sessions(session_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_browser_sessions_remote_session_id ON browser_sessions(remote_session_id);
CREATE INDEX IF NOT EXISTS idx_browser_sessions_bot_status ON browser_sessions(bot_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_browser_sessions_expires_at ON browser_sessions(expires_at);
