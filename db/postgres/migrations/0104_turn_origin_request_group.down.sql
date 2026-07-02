-- 0104_turn_origin_request_group
-- Rollback: drop turn provenance columns from bot_history_turns.

ALTER TABLE bot_history_turns
  DROP COLUMN IF EXISTS request_group_id,
  DROP COLUMN IF EXISTS origin_turn_id,
  DROP COLUMN IF EXISTS origin_kind;
