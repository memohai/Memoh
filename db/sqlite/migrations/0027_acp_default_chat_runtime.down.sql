-- 0027_acp_default_chat_runtime (down)
-- Remove bot default chat runtime fields and mode/runtime split columns.

CREATE TEMP TABLE IF NOT EXISTS _memoh_acp_default_chat_runtime_down_guard (
  ok INTEGER NOT NULL CHECK (ok = 1)
);

INSERT INTO _memoh_acp_default_chat_runtime_down_guard(ok)
SELECT 0 WHERE EXISTS (
  SELECT 1 FROM bots WHERE chat_runtime = 'acp_agent'
)
OR EXISTS (
  SELECT 1 FROM bot_sessions
  WHERE (runtime_type = 'acp_agent') != (type = 'acp_agent')
);

DROP TABLE _memoh_acp_default_chat_runtime_down_guard;

UPDATE bot_sessions
SET metadata = json_patch(
  COALESCE(NULLIF(metadata, ''), '{}'),
  COALESCE(NULLIF(runtime_metadata, ''), '{}')
)
WHERE type = 'acp_agent'
  AND runtime_type = 'acp_agent';

DROP INDEX IF EXISTS idx_bot_session_discuss_cursors_route;
DROP TABLE IF EXISTS bot_session_discuss_cursors;

DROP INDEX IF EXISTS idx_bot_sessions_bot_mode_runtime_active_updated;

DROP VIEW IF EXISTS bot_visible_history_messages;

ALTER TABLE bot_history_messages DROP COLUMN runtime_type;
ALTER TABLE bot_history_messages DROP COLUMN session_mode;

ALTER TABLE bot_sessions DROP COLUMN runtime_metadata;
ALTER TABLE bot_sessions DROP COLUMN runtime_type;
ALTER TABLE bot_sessions DROP COLUMN session_mode;

ALTER TABLE bots DROP COLUMN chat_acp_project_mode;
ALTER TABLE bots DROP COLUMN chat_acp_project_path;
ALTER TABLE bots DROP COLUMN chat_acp_agent_id;
ALTER TABLE bots DROP COLUMN chat_runtime;
