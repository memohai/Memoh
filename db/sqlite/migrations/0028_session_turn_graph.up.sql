-- 0028_session_turn_graph
-- Add immutable turn chains and session-level fork pointers for session history.

PRAGMA foreign_keys = OFF;

BEGIN;

-- Use the create/copy/drop/rename rebuild pattern instead of renaming the
-- original table first. SQLite may rewrite dependent child foreign keys when a
-- parent table is renamed, even with foreign_keys disabled.
CREATE TABLE bot_sessions_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_type TEXT,
  type TEXT NOT NULL DEFAULT 'chat' CHECK (type IN ('chat', 'heartbeat', 'schedule', 'subagent', 'discuss', 'acp_agent')),
  title TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  session_mode TEXT NOT NULL DEFAULT 'chat' CHECK (session_mode IN ('chat', 'discuss', 'heartbeat', 'schedule', 'subagent')),
  runtime_type TEXT NOT NULL DEFAULT 'model' CHECK (runtime_type IN ('model', 'acp_agent')),
  runtime_metadata TEXT NOT NULL DEFAULT '{}',
  default_head_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  forked_from_session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  forked_from_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  parent_session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);
INSERT INTO bot_sessions_new (
  id,
  bot_id,
  route_id,
  channel_type,
  type,
  title,
  metadata,
  session_mode,
  runtime_type,
  runtime_metadata,
  default_head_turn_id,
  forked_from_session_id,
  forked_from_turn_id,
  parent_session_id,
  created_by_user_id,
  created_at,
  updated_at,
  deleted_at
)
SELECT
  id,
  bot_id,
  route_id,
  channel_type,
  type,
  title,
  metadata,
  session_mode,
  runtime_type,
  runtime_metadata,
  NULL,
  NULL,
  NULL,
  parent_session_id,
  created_by_user_id,
  created_at,
  updated_at,
  deleted_at
FROM bot_sessions;
DROP TABLE bot_sessions;
ALTER TABLE bot_sessions_new RENAME TO bot_sessions;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_sessions_id_bot_unique ON bot_sessions(id, bot_id);

CREATE TABLE bot_history_messages_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  turn_message_seq INTEGER,
  sender_channel_identity_id TEXT REFERENCES channel_identities(id),
  sender_account_user_id TEXT REFERENCES users(id),
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
  content TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat' CHECK (session_mode IN ('chat', 'discuss', 'heartbeat', 'schedule', 'subagent')),
  runtime_type TEXT NOT NULL DEFAULT 'model' CHECK (runtime_type IN ('model', 'acp_agent')),
  model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  compact_id TEXT,
  event_id TEXT REFERENCES bot_session_events(id) ON DELETE SET NULL,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO bot_history_messages_new (
  id,
  bot_id,
  session_id,
  turn_id,
  turn_message_seq,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  model_id,
  compact_id,
  event_id,
  display_text,
  created_at
)
SELECT
  id,
  bot_id,
  session_id,
  NULL,
  NULL,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  model_id,
  compact_id,
  event_id,
  display_text,
  created_at
FROM bot_history_messages;
DROP TABLE bot_history_messages;
ALTER TABLE bot_history_messages_new RENAME TO bot_history_messages;

CREATE TABLE IF NOT EXISTS bot_history_turns (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  owner_session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  parent_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  request_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  final_assistant_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  -- Turn provenance: origin_kind/origin_turn_id record how the turn was
  -- created (message | retry | edit); request_group_id groups sibling turns
  -- carrying the same logical request (retry copies the source turn's group,
  -- send/edit start a new one). NULL request_group_id means the turn is its
  -- own group (read as COALESCE(request_group_id, id)).
  origin_kind TEXT,
  origin_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  request_group_id TEXT
);

CREATE TABLE tool_approval_requests_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  operation TEXT NOT NULL,
  tool_input TEXT NOT NULL,
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  decision_reason TEXT NOT NULL DEFAULT '',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  decided_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  requested_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  persist_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  decided_at TEXT,
  CONSTRAINT tool_approval_operation_check CHECK (operation IN ('read', 'write', 'exec')),
  CONSTRAINT tool_approval_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
  CONSTRAINT tool_approval_short_id_unique UNIQUE (session_id, short_id),
  FOREIGN KEY (session_id, bot_id) REFERENCES bot_sessions(id, bot_id) ON DELETE CASCADE
);
INSERT INTO tool_approval_requests_new (
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  operation,
  tool_input,
  short_id,
  status,
  decision_reason,
  requested_by_channel_identity_id,
  decided_by_channel_identity_id,
  requested_message_id,
  prompt_message_id,
  persist_turn_id,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  created_at,
  decided_at
)
SELECT
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  operation,
  tool_input,
  short_id,
  status,
  decision_reason,
  requested_by_channel_identity_id,
  decided_by_channel_identity_id,
  requested_message_id,
  prompt_message_id,
  NULL,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  created_at,
  decided_at
FROM tool_approval_requests;
DROP TABLE tool_approval_requests;
ALTER TABLE tool_approval_requests_new RENAME TO tool_approval_requests;

CREATE TABLE user_input_requests_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL DEFAULT 'ask_user',
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  input_json TEXT NOT NULL,
  ui_payload_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}',
  provider_metadata TEXT NOT NULL DEFAULT '{}',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  responded_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  assistant_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  tool_result_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  persist_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  expires_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at TEXT,
  canceled_at TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_input_tool_name_check CHECK (tool_name = 'ask_user'),
  CONSTRAINT user_input_status_check CHECK (status IN ('pending', 'submitted', 'canceled', 'expired', 'failed')),
  CONSTRAINT user_input_short_id_unique UNIQUE (session_id, short_id),
  FOREIGN KEY (session_id, bot_id) REFERENCES bot_sessions(id, bot_id) ON DELETE CASCADE
);
INSERT INTO user_input_requests_new (
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  short_id,
  status,
  input_json,
  ui_payload_json,
  result_json,
  provider_metadata,
  requested_by_channel_identity_id,
  responded_by_channel_identity_id,
  assistant_message_id,
  tool_result_message_id,
  prompt_message_id,
  persist_turn_id,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  expires_at,
  created_at,
  responded_at,
  canceled_at,
  updated_at
)
SELECT
  id,
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  short_id,
  status,
  input_json,
  ui_payload_json,
  result_json,
  provider_metadata,
  requested_by_channel_identity_id,
  responded_by_channel_identity_id,
  assistant_message_id,
  tool_result_message_id,
  prompt_message_id,
  NULL,
  prompt_external_message_id,
  source_platform,
  reply_target,
  conversation_type,
  expires_at,
  created_at,
  responded_at,
  canceled_at,
  updated_at
FROM user_input_requests;
DROP TABLE user_input_requests;
ALTER TABLE user_input_requests_new RENAME TO user_input_requests;

CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_id ON bot_sessions(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_route_id ON bot_sessions(route_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active ON bot_sessions(bot_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_parent ON bot_sessions(parent_session_id) WHERE parent_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_default_head_turn ON bot_sessions(default_head_turn_id) WHERE default_head_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_forked_from_session ON bot_sessions(forked_from_session_id) WHERE forked_from_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_forked_from_turn ON bot_sessions(forked_from_turn_id) WHERE forked_from_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_created_by_user_id ON bot_sessions(created_by_user_id) WHERE created_by_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_created_by ON bot_sessions(bot_id, created_by_user_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active_updated ON bot_sessions(bot_id, updated_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_mode_runtime_active_updated
  ON bot_sessions(bot_id, session_mode, runtime_type, updated_at DESC, id DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_bot_history_turns_bot_created ON bot_history_turns(bot_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_owner_session ON bot_history_turns(owner_session_id, created_at, id) WHERE owner_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_parent ON bot_history_turns(parent_turn_id) WHERE parent_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_request ON bot_history_turns(request_message_id) WHERE request_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_assistant ON bot_history_turns(final_assistant_message_id) WHERE final_assistant_message_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_turns_id_bot_unique ON bot_history_turns(id, bot_id);

CREATE TABLE IF NOT EXISTS bot_session_turn_heads (
  session_id TEXT NOT NULL,
  head_turn_id TEXT NOT NULL,
  bot_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (session_id, head_turn_id),
  FOREIGN KEY (session_id, bot_id) REFERENCES bot_sessions(id, bot_id) ON DELETE CASCADE,
  FOREIGN KEY (head_turn_id, bot_id) REFERENCES bot_history_turns(id, bot_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_bot_session_turn_heads_head
  ON bot_session_turn_heads(head_turn_id);
CREATE INDEX IF NOT EXISTS idx_bot_session_turn_heads_bot
  ON bot_session_turn_heads(bot_id);

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_compact ON bot_history_messages(compact_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session ON bot_history_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn ON bot_history_messages(turn_id, turn_message_seq, created_at) WHERE turn_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_source ON bot_history_messages(session_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_reply ON bot_history_messages(session_id, source_reply_to_message_id);

CREATE INDEX IF NOT EXISTS idx_tool_approval_bot_status_created ON tool_approval_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_session_status_created ON tool_approval_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_persist_turn ON tool_approval_requests(persist_turn_id) WHERE persist_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tool_approval_prompt_external ON tool_approval_requests(prompt_external_message_id) WHERE prompt_external_message_id != '';
CREATE UNIQUE INDEX IF NOT EXISTS tool_approval_tool_call_legacy_unique
  ON tool_approval_requests(session_id, tool_call_id)
  WHERE persist_turn_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS tool_approval_tool_call_turn_unique
  ON tool_approval_requests(session_id, tool_call_id, persist_turn_id)
  WHERE persist_turn_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_legacy_unique
  ON user_input_requests(session_id, tool_call_id)
  WHERE persist_turn_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_turn_unique
  ON user_input_requests(session_id, tool_call_id, persist_turn_id)
  WHERE persist_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_input_bot_status_created ON user_input_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_session_status_created ON user_input_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_user_input_persist_turn ON user_input_requests(persist_turn_id) WHERE persist_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_input_prompt_external ON user_input_requests(prompt_external_message_id) WHERE prompt_external_message_id != '';

CREATE TEMP TABLE session_turn_seed (
  turn_id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  turn_group INTEGER NOT NULL,
  turn_pos INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  request_message_id TEXT,
  final_assistant_message_id TEXT
);

INSERT INTO session_turn_seed (
  turn_id,
  bot_id,
  session_id,
  turn_group,
  turn_pos,
  created_at,
  request_message_id,
  final_assistant_message_id
)
WITH ordered AS (
  SELECT
    m.id,
    m.bot_id,
    m.session_id,
    m.role,
    m.created_at,
    ROW_NUMBER() OVER (PARTITION BY m.session_id ORDER BY m.created_at, m.id) AS message_seq,
    SUM(CASE WHEN m.role = 'user' THEN 1 ELSE 0 END) OVER (
      PARTITION BY m.session_id
      ORDER BY m.created_at, m.id
      ROWS UNBOUNDED PRECEDING
    ) AS user_group
  FROM bot_history_messages m
  WHERE m.session_id IS NOT NULL
    AND m.turn_id IS NULL
),
grouped AS (
  SELECT
    *,
    CASE WHEN user_group = 0 THEN -message_seq ELSE user_group END AS turn_group
  FROM ordered
),
turn_aggregates AS (
  SELECT
    g.bot_id,
    g.session_id,
    g.turn_group,
    MIN(g.created_at) AS created_at,
    MIN(CASE WHEN g.role = 'user' THEN g.id END) AS request_message_id,
    (
      SELECT g2.id
      FROM grouped g2
      WHERE g2.session_id = g.session_id
        AND g2.turn_group = g.turn_group
        AND g2.role = 'assistant'
      ORDER BY g2.created_at DESC, g2.id DESC
      LIMIT 1
    ) AS final_assistant_message_id
  FROM grouped g
  GROUP BY g.bot_id, g.session_id, g.turn_group
),
numbered AS (
  SELECT
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-' ||
    '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6))) AS turn_id,
    *,
    ROW_NUMBER() OVER (
      PARTITION BY session_id
      ORDER BY created_at, COALESCE(request_message_id, final_assistant_message_id, ''), turn_group
    ) AS turn_pos
  FROM turn_aggregates
)
SELECT
  turn_id,
  bot_id,
  session_id,
  turn_group,
  turn_pos,
  created_at,
  request_message_id,
  final_assistant_message_id
FROM numbered;

INSERT INTO bot_history_turns (
  id,
  bot_id,
  owner_session_id,
  parent_turn_id,
  request_message_id,
  final_assistant_message_id,
  created_at,
  updated_at
)
SELECT
  current.turn_id,
  current.bot_id,
  current.session_id,
  previous.turn_id,
  current.request_message_id,
  current.final_assistant_message_id,
  current.created_at,
  current.created_at
FROM session_turn_seed current
LEFT JOIN session_turn_seed previous
  ON previous.session_id = current.session_id
 AND previous.turn_pos = current.turn_pos - 1;

WITH ordered AS (
  SELECT
    m.id,
    m.session_id,
    m.created_at,
    ROW_NUMBER() OVER (PARTITION BY m.session_id ORDER BY m.created_at, m.id) AS message_seq,
    SUM(CASE WHEN m.role = 'user' THEN 1 ELSE 0 END) OVER (
      PARTITION BY m.session_id
      ORDER BY m.created_at, m.id
      ROWS UNBOUNDED PRECEDING
    ) AS user_group
  FROM bot_history_messages m
  WHERE m.session_id IS NOT NULL
    AND m.turn_id IS NULL
),
grouped AS (
  SELECT
    *,
    CASE WHEN user_group = 0 THEN -message_seq ELSE user_group END AS turn_group
  FROM ordered
),
mapped AS (
  SELECT
    g.id AS message_id,
    seed.turn_id,
    ROW_NUMBER() OVER (PARTITION BY g.session_id, g.turn_group ORDER BY g.created_at, g.id) AS turn_message_seq
  FROM grouped g
  JOIN session_turn_seed seed
    ON seed.session_id = g.session_id
   AND seed.turn_group = g.turn_group
)
UPDATE bot_history_messages
SET turn_id = (SELECT mapped.turn_id FROM mapped WHERE mapped.message_id = bot_history_messages.id),
    turn_message_seq = (SELECT mapped.turn_message_seq FROM mapped WHERE mapped.message_id = bot_history_messages.id)
WHERE id IN (SELECT message_id FROM mapped);

UPDATE bot_sessions
SET default_head_turn_id = (
  SELECT seed.turn_id
  FROM session_turn_seed seed
  WHERE seed.session_id = bot_sessions.id
  ORDER BY seed.turn_pos DESC
  LIMIT 1
)
WHERE default_head_turn_id IS NULL
  AND EXISTS (
    SELECT 1
    FROM session_turn_seed seed
    WHERE seed.session_id = bot_sessions.id
  );

INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id)
SELECT s.id, s.default_head_turn_id, s.bot_id
FROM bot_sessions s
JOIN bot_history_turns t
  ON t.id = s.default_head_turn_id
 AND t.bot_id = s.bot_id
WHERE s.default_head_turn_id IS NOT NULL
ON CONFLICT (session_id, head_turn_id) DO NOTHING;

UPDATE bot_sessions
SET default_head_turn_id = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE default_head_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = bot_sessions.default_head_turn_id
      AND t.bot_id = bot_sessions.bot_id
  );

UPDATE bot_sessions
SET forked_from_turn_id = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE forked_from_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = bot_sessions.forked_from_turn_id
      AND t.bot_id = bot_sessions.bot_id
  );

CREATE TRIGGER IF NOT EXISTS bot_sessions_default_head_same_bot_insert
BEFORE INSERT ON bot_sessions
FOR EACH ROW
WHEN NEW.default_head_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.default_head_turn_id
      AND t.bot_id = NEW.bot_id
  )
BEGIN
  SELECT RAISE(ABORT, 'bot_sessions.default_head_turn_id must reference a turn from the same bot');
END;

CREATE TRIGGER IF NOT EXISTS bot_sessions_default_head_same_bot_update
BEFORE UPDATE OF default_head_turn_id, bot_id ON bot_sessions
FOR EACH ROW
WHEN NEW.default_head_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.default_head_turn_id
      AND t.bot_id = NEW.bot_id
  )
BEGIN
  SELECT RAISE(ABORT, 'bot_sessions.default_head_turn_id must reference a turn from the same bot');
END;

CREATE TRIGGER IF NOT EXISTS bot_sessions_forked_from_turn_same_bot_insert
BEFORE INSERT ON bot_sessions
FOR EACH ROW
WHEN NEW.forked_from_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.forked_from_turn_id
      AND t.bot_id = NEW.bot_id
  )
BEGIN
  SELECT RAISE(ABORT, 'bot_sessions.forked_from_turn_id must reference a turn from the same bot');
END;

CREATE TRIGGER IF NOT EXISTS bot_sessions_forked_from_turn_same_bot_update
BEFORE UPDATE OF forked_from_turn_id, bot_id ON bot_sessions
FOR EACH ROW
WHEN NEW.forked_from_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.forked_from_turn_id
      AND t.bot_id = NEW.bot_id
  )
BEGIN
  SELECT RAISE(ABORT, 'bot_sessions.forked_from_turn_id must reference a turn from the same bot');
END;

CREATE TRIGGER IF NOT EXISTS tool_approval_persist_turn_owner_insert
BEFORE INSERT ON tool_approval_requests
FOR EACH ROW
WHEN NEW.persist_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.persist_turn_id
      AND t.bot_id = NEW.bot_id
      AND t.owner_session_id = NEW.session_id
  )
BEGIN
  SELECT RAISE(ABORT, 'persist_turn_id must reference a turn from the same bot session');
END;

CREATE TRIGGER IF NOT EXISTS tool_approval_persist_turn_owner_update
BEFORE UPDATE OF persist_turn_id, bot_id, session_id ON tool_approval_requests
FOR EACH ROW
WHEN NEW.persist_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.persist_turn_id
      AND t.bot_id = NEW.bot_id
      AND t.owner_session_id = NEW.session_id
  )
BEGIN
  SELECT RAISE(ABORT, 'persist_turn_id must reference a turn from the same bot session');
END;

CREATE TRIGGER IF NOT EXISTS user_input_persist_turn_owner_insert
BEFORE INSERT ON user_input_requests
FOR EACH ROW
WHEN NEW.persist_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.persist_turn_id
      AND t.bot_id = NEW.bot_id
      AND t.owner_session_id = NEW.session_id
  )
BEGIN
  SELECT RAISE(ABORT, 'persist_turn_id must reference a turn from the same bot session');
END;

CREATE TRIGGER IF NOT EXISTS user_input_persist_turn_owner_update
BEFORE UPDATE OF persist_turn_id, bot_id, session_id ON user_input_requests
FOR EACH ROW
WHEN NEW.persist_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.persist_turn_id
      AND t.bot_id = NEW.bot_id
      AND t.owner_session_id = NEW.session_id
  )
BEGIN
  SELECT RAISE(ABORT, 'persist_turn_id must reference a turn from the same bot session');
END;

DROP TABLE session_turn_seed;
COMMIT;

PRAGMA foreign_keys = ON;
