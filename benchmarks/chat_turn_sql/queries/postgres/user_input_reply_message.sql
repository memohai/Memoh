-- Benchmark scenario: user_input_reply_message
-- Production query: db/postgres/queries/user_input.sql GetPendingUserInputByReplyMessage
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = $2::uuid
    AND s.deleted_at IS NULL
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = $1::uuid
  AND uir.session_id = $2::uuid
  AND uir.prompt_external_message_id = $3::text
  AND uir.status = 'pending'
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY uir.created_at DESC
LIMIT 1;
