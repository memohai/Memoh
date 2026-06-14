-- 0021_session_branches
-- Add in-session branch paths for fork/edit-and-rerun chat history.

PRAGMA foreign_keys = OFF;

BEGIN;

-- Older SQLite session-type rebuilds renamed bot_sessions to bot_sessions_old.
-- SQLite 3.26+ rewrites dependent foreign keys during that rename, so existing
-- installs can carry child-table FKs pointing at the dropped temporary table.
-- Repair those schema references before touching bot_sessions again.
PRAGMA writable_schema = ON;

UPDATE sqlite_schema
SET sql = replace(sql, 'bot_sessions_old', 'bot_sessions')
WHERE sql LIKE '%bot_sessions_old%';

PRAGMA writable_schema = OFF;
PRAGMA schema_version = 1000021;

CREATE TABLE IF NOT EXISTS bot_sessions_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_type TEXT,
  type TEXT NOT NULL DEFAULT 'chat' CHECK (type IN ('chat', 'heartbeat', 'schedule', 'subagent', 'discuss', 'acp_agent')),
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  active_branch_id TEXT,
  parent_session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);

INSERT INTO bot_sessions_new (
  id, bot_id, route_id, channel_type, type, title, metadata, active_branch_id,
  parent_session_id, created_by_user_id, created_at, updated_at, deleted_at
)
SELECT
  id, bot_id, route_id, channel_type, type, title, metadata, NULL,
  parent_session_id, created_by_user_id, created_at, updated_at, deleted_at
FROM bot_sessions;

DROP TABLE bot_sessions;
ALTER TABLE bot_sessions_new RENAME TO bot_sessions;

CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_id ON bot_sessions(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_route_id ON bot_sessions(route_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active ON bot_sessions(bot_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_parent ON bot_sessions(parent_session_id) WHERE parent_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_created_by_user_id ON bot_sessions(created_by_user_id) WHERE created_by_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_created_by ON bot_sessions(bot_id, created_by_user_id, deleted_at);

CREATE TABLE IF NOT EXISTS bot_session_branches (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  parent_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  fork_from_message_id TEXT,
  fork_from_seq INTEGER,
  title TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_session_branches_root
  ON bot_session_branches(session_id)
  WHERE parent_branch_id IS NULL AND fork_from_message_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_bot_session_branches_session ON bot_session_branches(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_session_branches_parent ON bot_session_branches(parent_branch_id) WHERE parent_branch_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS bot_history_messages_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  branch_seq INTEGER,
  sender_channel_identity_id TEXT REFERENCES channel_identities(id),
  sender_account_user_id TEXT REFERENCES users(id),
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
  content TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  compact_id TEXT,
  event_id TEXT REFERENCES bot_session_events(id) ON DELETE SET NULL,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bot_history_messages_new (
  id, bot_id, session_id, branch_id, branch_seq, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata, usage, model_id,
  compact_id, event_id, display_text, created_at
)
SELECT
  id, bot_id, session_id, NULL, NULL, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata, usage, model_id,
  compact_id, event_id, display_text, created_at
FROM bot_history_messages;

DROP TABLE bot_history_messages;
ALTER TABLE bot_history_messages_new RENAME TO bot_history_messages;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_compact ON bot_history_messages(compact_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session
  ON bot_history_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_source
  ON bot_history_messages(session_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_reply
  ON bot_history_messages(session_id, source_reply_to_message_id);

INSERT INTO bot_session_branches (id, session_id, created_at, updated_at)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  s.id,
  COALESCE(MIN(m.created_at), s.created_at),
  COALESCE(MAX(m.created_at), s.updated_at)
FROM bot_sessions s
LEFT JOIN bot_history_messages m ON m.session_id = s.id
WHERE NOT EXISTS (
  SELECT 1
  FROM bot_session_branches b
  WHERE b.session_id = s.id
    AND b.parent_branch_id IS NULL
    AND b.fork_from_message_id IS NULL
)
GROUP BY s.id, s.created_at, s.updated_at;

UPDATE bot_sessions
SET active_branch_id = (
  SELECT b.id
  FROM bot_session_branches b
  WHERE b.session_id = bot_sessions.id
    AND b.parent_branch_id IS NULL
    AND b.fork_from_message_id IS NULL
  ORDER BY b.created_at ASC
  LIMIT 1
)
WHERE active_branch_id IS NULL;

WITH numbered AS (
  SELECT
    m.id AS message_id,
    b.id AS branch_id,
    ROW_NUMBER() OVER (PARTITION BY m.session_id ORDER BY m.created_at ASC, m.id ASC) AS branch_seq
  FROM bot_history_messages m
  JOIN bot_session_branches b ON b.session_id = m.session_id
  WHERE b.parent_branch_id IS NULL
    AND b.fork_from_message_id IS NULL
    AND m.session_id IS NOT NULL
    AND (m.branch_id IS NULL OR m.branch_seq IS NULL)
)
UPDATE bot_history_messages
SET branch_id = (SELECT numbered.branch_id FROM numbered WHERE numbered.message_id = bot_history_messages.id),
    branch_seq = (SELECT numbered.branch_seq FROM numbered WHERE numbered.message_id = bot_history_messages.id)
WHERE id IN (SELECT message_id FROM numbered);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_branch_seq
  ON bot_history_messages(branch_id, branch_seq)
  WHERE branch_id IS NOT NULL AND branch_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_branch
  ON bot_history_messages(branch_id, branch_seq);

COMMIT;

PRAGMA foreign_keys = ON;
