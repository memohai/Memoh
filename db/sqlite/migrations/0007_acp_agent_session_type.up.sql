-- 0006_acp_agent_session_type
-- Add acp_agent as a first-class bot session type for dedicated ACP sessions.

PRAGMA foreign_keys = OFF;
PRAGMA legacy_alter_table = ON;

BEGIN;

ALTER TABLE bot_sessions RENAME TO bot_sessions_old;

CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_type TEXT,
  type TEXT NOT NULL DEFAULT 'chat' CHECK (type IN ('chat', 'heartbeat', 'schedule', 'subagent', 'discuss', 'acp_agent')),
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  parent_session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);

INSERT INTO bot_sessions (
  id, bot_id, route_id, channel_type, type, title, metadata,
  parent_session_id, created_at, updated_at, deleted_at
)
SELECT
  id, bot_id, route_id, channel_type, type, title, metadata,
  parent_session_id, created_at, updated_at, deleted_at
FROM bot_sessions_old;

DROP TABLE bot_sessions_old;

CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_id ON bot_sessions(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_route_id ON bot_sessions(route_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active ON bot_sessions(bot_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_parent ON bot_sessions(parent_session_id) WHERE parent_session_id IS NOT NULL;

COMMIT;

PRAGMA legacy_alter_table = OFF;
PRAGMA foreign_keys = ON;
