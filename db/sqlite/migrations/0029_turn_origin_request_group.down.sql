-- 0029_turn_origin_request_group
-- Rollback: rebuild bot_history_turns without the provenance columns.

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
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
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

COMMIT;

PRAGMA foreign_keys = ON;
