-- Benchmark scenario: head_resolve
-- Production query: db/postgres/queries/messages.sql ResolveSessionTurnHead
WITH RECURSIVE descendant_turns AS (
  SELECT t.id
  FROM bot_history_turns t
  WHERE t.id = $2::uuid
  UNION ALL
  SELECT c.id
  FROM bot_history_turns c
  JOIN descendant_turns dt ON c.parent_turn_id = dt.id
)
SELECT h.head_turn_id
FROM bot_session_turn_heads h
WHERE h.session_id = $1::uuid
  AND h.head_turn_id IN (SELECT dt.id FROM descendant_turns dt)
ORDER BY h.updated_at DESC, h.head_turn_id DESC
LIMIT 1;
