-- Benchmark scenario: turn_path
-- Production query: db/postgres/queries/messages.sql ListSessionTurnPathIDs
WITH RECURSIVE path_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_history_turns t
  WHERE t.id = $1::uuid
  UNION ALL
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN path_turns pt ON pt.parent_turn_id = p.id
)
SELECT pt.id
FROM path_turns pt;
