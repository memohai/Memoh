-- 0023_repair_branch_turn_columns
-- Rebuild tables affected by the pre-turn SQLite writable_schema column insertion.

PRAGMA foreign_keys = OFF;

BEGIN;

DROP INDEX IF EXISTS idx_bot_session_branches_root;
DROP INDEX IF EXISTS idx_bot_session_branches_session;
DROP INDEX IF EXISTS idx_bot_session_branches_parent;
DROP INDEX IF EXISTS idx_bot_history_messages_bot_created;
DROP INDEX IF EXISTS idx_bot_history_messages_compact;
DROP INDEX IF EXISTS idx_bot_history_messages_session;
DROP INDEX IF EXISTS idx_bot_history_messages_branch_seq;
DROP INDEX IF EXISTS idx_bot_history_messages_branch;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_messages_session_source;
DROP INDEX IF EXISTS idx_bot_history_messages_session_reply;

CREATE TABLE bot_session_branches_0023_new (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  parent_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  fork_from_message_id TEXT,
  fork_from_seq INTEGER,
  fork_from_turn_id TEXT,
  fork_from_turn_seq INTEGER,
  title TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bot_session_branches_0023_new (
  id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq,
  fork_from_turn_id, fork_from_turn_seq, title, created_at, updated_at
)
SELECT
  id,
  session_id,
  parent_branch_id,
  fork_from_message_id,
  fork_from_seq,
  CASE
    WHEN fork_from_turn_id LIKE '____-__-__ __:__:__'
      OR fork_from_turn_seq LIKE '____-__-__ __:__:__'
      OR title LIKE '____-__-__ __:__:__'
      OR created_at IS NULL OR created_at = ''
      OR updated_at IS NULL OR updated_at = ''
      THEN NULL
    ELSE fork_from_turn_id
  END,
  CASE
    WHEN fork_from_turn_id LIKE '____-__-__ __:__:__'
      OR fork_from_turn_seq LIKE '____-__-__ __:__:__'
      OR title LIKE '____-__-__ __:__:__'
      OR created_at IS NULL OR created_at = ''
      OR updated_at IS NULL OR updated_at = ''
      THEN NULL
    WHEN typeof(fork_from_turn_seq) IN ('integer', 'real') THEN CAST(fork_from_turn_seq AS INTEGER)
    WHEN typeof(fork_from_turn_seq) = 'text'
      AND fork_from_turn_seq != ''
      AND fork_from_turn_seq NOT GLOB '*[^0-9]*'
      THEN CAST(fork_from_turn_seq AS INTEGER)
    ELSE NULL
  END,
  CASE
    WHEN fork_from_turn_id LIKE '____-__-__ __:__:__'
      OR fork_from_turn_seq LIKE '____-__-__ __:__:__'
      OR title LIKE '____-__-__ __:__:__'
      OR created_at IS NULL OR created_at = ''
      OR updated_at IS NULL OR updated_at = ''
      THEN CASE
        WHEN fork_from_turn_id IS NOT NULL
          AND fork_from_turn_id != ''
          AND fork_from_turn_id NOT LIKE '____-__-__ __:__:__'
          THEN fork_from_turn_id
        ELSE NULL
      END
    ELSE title
  END,
  COALESCE(
    NULLIF(CASE WHEN fork_from_turn_seq LIKE '____-__-__ __:__:__' THEN fork_from_turn_seq END, ''),
    NULLIF(CASE WHEN fork_from_turn_id LIKE '____-__-__ __:__:__' THEN fork_from_turn_id END, ''),
    NULLIF(created_at, ''),
    CURRENT_TIMESTAMP
  ),
  COALESCE(
    NULLIF(CASE WHEN title LIKE '____-__-__ __:__:__' THEN title END, ''),
    NULLIF(CASE WHEN fork_from_turn_seq LIKE '____-__-__ __:__:__' THEN fork_from_turn_seq END, ''),
    NULLIF(CASE WHEN fork_from_turn_id LIKE '____-__-__ __:__:__' THEN fork_from_turn_id END, ''),
    NULLIF(updated_at, ''),
    NULLIF(created_at, ''),
    CURRENT_TIMESTAMP
  )
FROM bot_session_branches;

DROP TABLE bot_session_branches;
ALTER TABLE bot_session_branches_0023_new RENAME TO bot_session_branches;

CREATE TABLE bot_history_messages_0023_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  branch_seq INTEGER,
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
  model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  compact_id TEXT,
  event_id TEXT REFERENCES bot_session_events(id) ON DELETE SET NULL,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bot_history_messages_0023_new (
  id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq,
  sender_channel_identity_id, sender_account_user_id, source_message_id,
  source_reply_to_message_id, role, content, metadata, usage, model_id,
  compact_id, event_id, display_text, created_at
)
SELECT
  id,
  bot_id,
  session_id,
  branch_id,
  branch_seq,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN turn_id ELSE NULL END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN turn_message_seq ELSE NULL END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN sender_channel_identity_id ELSE NULLIF(turn_id, '') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN sender_account_user_id ELSE NULLIF(turn_message_seq, '') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN source_message_id ELSE NULLIF(sender_channel_identity_id, '') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN source_reply_to_message_id ELSE NULLIF(sender_account_user_id, '') END,
  CASE
    WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN role
    WHEN source_message_id IN ('user', 'assistant', 'system', 'tool') THEN source_message_id
    ELSE 'user'
  END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN content ELSE COALESCE(NULLIF(source_reply_to_message_id, ''), '{}') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN metadata ELSE COALESCE(NULLIF(role, ''), '{}') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN usage ELSE NULLIF(content, '') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN model_id ELSE NULLIF(metadata, '') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN compact_id ELSE NULLIF(usage, '') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN event_id ELSE NULLIF(model_id, '') END,
  CASE WHEN role IN ('user', 'assistant', 'system', 'tool') AND created_at IS NOT NULL AND created_at != '' THEN display_text ELSE NULLIF(compact_id, '') END,
  COALESCE(
    NULLIF(CASE WHEN role IN ('user', 'assistant', 'system', 'tool') THEN created_at ELSE event_id END, ''),
    CURRENT_TIMESTAMP
  )
FROM bot_history_messages;

DROP TABLE bot_history_messages;
ALTER TABLE bot_history_messages_0023_new RENAME TO bot_history_messages;

WITH turn_ranges AS (
  SELECT
    t.id AS turn_id,
    t.branch_id,
    req.branch_seq AS start_seq,
    COALESCE((
      SELECT MIN(next_req.branch_seq)
      FROM bot_history_messages next_req
      WHERE next_req.branch_id = t.branch_id
        AND next_req.role = 'user'
        AND next_req.branch_seq > req.branch_seq
    ), 9223372036854775807) AS end_seq
  FROM bot_history_turns t
  JOIN bot_history_messages req ON req.id = t.request_message_id
)
UPDATE bot_history_messages
SET turn_id = (SELECT tr.turn_id FROM turn_ranges tr WHERE bot_history_messages.branch_id = tr.branch_id AND bot_history_messages.branch_seq >= tr.start_seq AND bot_history_messages.branch_seq < tr.end_seq),
    turn_message_seq = (SELECT bot_history_messages.branch_seq - tr.start_seq + 1 FROM turn_ranges tr WHERE bot_history_messages.branch_id = tr.branch_id AND bot_history_messages.branch_seq >= tr.start_seq AND bot_history_messages.branch_seq < tr.end_seq)
WHERE turn_id IS NULL
  AND EXISTS (
    SELECT 1 FROM turn_ranges tr
    WHERE bot_history_messages.branch_id = tr.branch_id
      AND bot_history_messages.branch_seq >= tr.start_seq
      AND bot_history_messages.branch_seq < tr.end_seq
  );

UPDATE bot_session_branches
SET fork_from_turn_id = (
      SELECT t.id
      FROM bot_history_turns t
      WHERE t.branch_id = bot_session_branches.parent_branch_id
        AND t.final_assistant_message_id = bot_session_branches.fork_from_message_id
      ORDER BY t.turn_seq DESC, t.created_at DESC
      LIMIT 1
    ),
    fork_from_turn_seq = COALESCE((
      SELECT t.turn_seq
      FROM bot_history_turns t
      WHERE t.branch_id = bot_session_branches.parent_branch_id
        AND t.final_assistant_message_id = bot_session_branches.fork_from_message_id
      ORDER BY t.turn_seq DESC, t.created_at DESC
      LIMIT 1
    ), fork_from_seq)
WHERE fork_from_message_id IS NOT NULL
  AND parent_branch_id IS NOT NULL
  AND (fork_from_turn_id IS NULL OR fork_from_turn_seq IS NULL);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_session_branches_root
  ON bot_session_branches(session_id)
  WHERE parent_branch_id IS NULL AND fork_from_message_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_bot_session_branches_session
  ON bot_session_branches(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_session_branches_parent
  ON bot_session_branches(parent_branch_id) WHERE parent_branch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created
  ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_compact
  ON bot_history_messages(compact_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session
  ON bot_history_messages(session_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_branch_seq
  ON bot_history_messages(branch_id, branch_seq)
  WHERE branch_id IS NOT NULL AND branch_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_branch
  ON bot_history_messages(branch_id, branch_seq);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_source
  ON bot_history_messages(session_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_reply
  ON bot_history_messages(session_id, source_reply_to_message_id);

COMMIT;

PRAGMA foreign_keys = ON;
