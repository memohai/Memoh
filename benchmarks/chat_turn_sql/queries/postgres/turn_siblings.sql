-- Benchmark scenario: turn_siblings
-- Production query: db/postgres/queries/messages.sql ListSessionTurnSiblings
WITH RECURSIVE session_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_session_turn_heads h
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE h.session_id = $1::uuid
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN session_turns st ON st.parent_turn_id = p.id
),
page_parents AS (
  SELECT DISTINCT t.parent_turn_id
  FROM bot_history_turns t
  WHERE t.id = ANY($2::uuid[])
)
SELECT
  t.id AS turn_id,
  t.parent_turn_id,
  COALESCE(t.request_group_id, t.id) AS request_group_id,
  (t.request_message_id IS NOT NULL)::boolean AS has_user,
  (t.final_assistant_message_id IS NOT NULL)::boolean AS has_assistant
FROM bot_history_turns t
JOIN session_turns st ON st.id = t.id
WHERE (
    t.parent_turn_id IS NOT NULL
    AND t.parent_turn_id IN (
      SELECT pp.parent_turn_id FROM page_parents pp WHERE pp.parent_turn_id IS NOT NULL
    )
  )
  OR (
    t.parent_turn_id IS NULL
    AND EXISTS (
      SELECT 1 FROM page_parents pp WHERE pp.parent_turn_id IS NULL
    )
  )
ORDER BY t.created_at ASC, t.id ASC;
