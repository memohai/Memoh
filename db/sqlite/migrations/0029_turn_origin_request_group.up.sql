-- 0029_turn_origin_request_group
-- Materialize turn provenance on bot_history_turns: origin_kind/origin_turn_id
-- record how a turn was created (message | retry | edit), and request_group_id
-- groups sibling turns that carry the same logical request (retry copies the
-- source turn's group, send/edit start a new group). NULL request_group_id
-- means the turn is its own group (COALESCE(request_group_id, id)).
--
-- SQLite has no ADD COLUMN IF NOT EXISTS, and the 0001 baseline already carries
-- these columns for fresh installs, so rebuild the table instead of ALTERing:
-- a fresh replay recreates the same shape, an existing install gains the
-- columns. The explicit INSERT column list works for both.

PRAGMA foreign_keys = OFF;

BEGIN;

-- Triggers on other tables reference bot_history_turns; SQLite re-parses them
-- during the RENAME below and errors out if the table is missing, so drop
-- them first and recreate them after the rebuild.
DROP TRIGGER IF EXISTS bot_sessions_default_head_same_bot_insert;
DROP TRIGGER IF EXISTS bot_sessions_default_head_same_bot_update;
DROP TRIGGER IF EXISTS bot_sessions_forked_from_turn_same_bot_insert;
DROP TRIGGER IF EXISTS bot_sessions_forked_from_turn_same_bot_update;
DROP TRIGGER IF EXISTS tool_approval_persist_turn_owner_insert;
DROP TRIGGER IF EXISTS tool_approval_persist_turn_owner_update;
DROP TRIGGER IF EXISTS user_input_persist_turn_owner_insert;
DROP TRIGGER IF EXISTS user_input_persist_turn_owner_update;

CREATE TABLE bot_history_turns_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  owner_session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  parent_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  request_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  final_assistant_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  origin_kind TEXT,
  origin_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  request_group_id TEXT
);

INSERT INTO bot_history_turns_new (
  id, bot_id, owner_session_id, parent_turn_id,
  request_message_id, final_assistant_message_id, created_at, updated_at
)
SELECT
  id, bot_id, owner_session_id, parent_turn_id,
  request_message_id, final_assistant_message_id, created_at, updated_at
FROM bot_history_turns;

DROP TABLE bot_history_turns;
ALTER TABLE bot_history_turns_new RENAME TO bot_history_turns;

CREATE INDEX IF NOT EXISTS idx_bot_history_turns_bot_created
  ON bot_history_turns(bot_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_owner_session
  ON bot_history_turns(owner_session_id, created_at, id)
  WHERE owner_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_parent
  ON bot_history_turns(parent_turn_id)
  WHERE parent_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_request
  ON bot_history_turns(request_message_id) WHERE request_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_assistant
  ON bot_history_turns(final_assistant_message_id) WHERE final_assistant_message_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_turns_id_bot_unique
  ON bot_history_turns(id, bot_id);

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

-- Backfill legacy sibling groups with the same request fingerprint the old
-- graph endpoint hashed at read time (request text + ordered asset key), so
-- pre-existing retry variants keep grouping together. Turns without a request
-- message stay NULL (each is its own group). Group id = earliest turn in the
-- group (backfilled leaders may point at themselves; COALESCE semantics are
-- unaffected). Uses a scratch table because sqlc's SQLite grammar cannot parse
-- UPDATE ... FROM.
CREATE TABLE turn_request_group_backfill (
  turn_id TEXT PRIMARY KEY,
  parent_key TEXT NOT NULL,
  request_text_key TEXT NOT NULL,
  asset_key TEXT NOT NULL,
  created_at TEXT NOT NULL
);

INSERT INTO turn_request_group_backfill (turn_id, parent_key, request_text_key, asset_key, created_at)
SELECT
  t.id,
  COALESCE(t.parent_turn_id, ''),
  COALESCE(NULLIF(TRIM(COALESCE(rm.display_text, '')), ''), rm.content),
  COALESCE((
    SELECT GROUP_CONCAT(
      COALESCE(a.content_hash, '') || ':' ||
      COALESCE(a.name, '') || ':' ||
      COALESCE(a.role, '') || ':' ||
      COALESCE(CAST(a.ordinal AS TEXT), ''),
      '|'
    )
    FROM (
      SELECT a2.id, a2.message_id, a2.role, a2.ordinal, a2.content_hash, a2.name
      FROM bot_history_message_assets a2
      WHERE a2.message_id = t.request_message_id
      ORDER BY a2.content_hash, a2.name, a2.role, a2.ordinal, a2.id
    ) a
  ), ''),
  t.created_at
FROM bot_history_turns t
JOIN bot_history_messages rm ON rm.id = t.request_message_id
WHERE t.request_group_id IS NULL;

UPDATE bot_history_turns
SET request_group_id = (
  SELECT leader.turn_id
  FROM turn_request_group_backfill self
  JOIN turn_request_group_backfill leader
    ON leader.parent_key = self.parent_key
   AND leader.request_text_key = self.request_text_key
   AND leader.asset_key = self.asset_key
  WHERE self.turn_id = bot_history_turns.id
  ORDER BY leader.created_at, leader.turn_id
  LIMIT 1
)
WHERE id IN (
  SELECT self.turn_id
  FROM turn_request_group_backfill self
  JOIN turn_request_group_backfill other
    ON other.parent_key = self.parent_key
   AND other.request_text_key = self.request_text_key
   AND other.asset_key = self.asset_key
   AND other.turn_id <> self.turn_id
);

DROP TABLE turn_request_group_backfill;

COMMIT;

PRAGMA foreign_keys = ON;
