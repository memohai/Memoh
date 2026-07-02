-- Benchmark scenario: turn_graph
-- Production query: db/postgres/queries/messages.sql ListSessionTurnGraphTurns
WITH RECURSIVE graph_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = $1::uuid
    AND s.deleted_at IS NULL
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN graph_turns gt ON gt.parent_turn_id = p.id
)
SELECT t.*
FROM graph_turns gt
JOIN bot_history_turns t ON t.id = gt.id
ORDER BY t.created_at ASC, t.id ASC;
