-- 0030_session_turn_position_allocator
-- Remove the per-session turn position allocator.

-- Keep full rollback atomic with the older ACP session-type guard. If acp_agent
-- sessions exist, migration 0007 down will fail, so fail before changing
-- bot_sessions here.
CREATE TEMP TABLE IF NOT EXISTS _memoh_acp_session_type_down_guard (
  ok INTEGER NOT NULL CHECK (ok = 1)
);

INSERT INTO _memoh_acp_session_type_down_guard(ok)
SELECT 0 WHERE EXISTS (SELECT 1 FROM bot_sessions WHERE type = 'acp_agent');

DROP TABLE _memoh_acp_session_type_down_guard;

ALTER TABLE bot_sessions
  DROP COLUMN next_turn_position;
