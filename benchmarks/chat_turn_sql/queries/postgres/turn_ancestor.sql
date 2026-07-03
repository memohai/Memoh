-- Benchmark scenario: turn_ancestor
-- Production query: db/postgres/queries/messages.sql GetSessionTurnAncestorMatch
-- Args: $1 ancestor_turn_id, $2 turn_id
WITH RECURSIVE path_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_history_turns t
  WHERE t.id = $2::uuid
  UNION ALL
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN path_turns pt ON pt.parent_turn_id = p.id
  WHERE pt.id <> $1::uuid
)
SELECT pt.id
FROM path_turns pt
JOIN bot_history_turns matched ON matched.id = pt.id
  AND matched.id = $1::uuid
LIMIT 1;
