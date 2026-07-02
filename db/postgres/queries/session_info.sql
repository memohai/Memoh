-- name: CountMessagesBySession :one
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT COUNT(*)::bigint AS message_count
FROM bot_history_messages m
WHERE m.turn_id IN (SELECT id FROM visible_turns);

-- name: GetLatestAssistantUsage :one
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  COALESCE((m.usage->>'inputTokens')::bigint, 0)::bigint AS input_tokens
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
WHERE m.role = 'assistant'
  AND m.usage IS NOT NULL
ORDER BY vt.depth ASC, COALESCE(m.turn_message_seq, 9223372036854775807) DESC, m.created_at DESC
LIMIT 1;

-- name: GetSessionCacheStats :one
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  COALESCE(SUM((m.usage->>'inputTokens')::bigint), 0)::bigint AS total_input_tokens,
  COALESCE(SUM((m.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint), 0)::bigint AS cache_read_tokens
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
WHERE m.usage IS NOT NULL;

-- name: GetLatestSessionIDByBot :one
SELECT s.id
FROM bot_sessions s
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.type = 'chat'
  AND s.deleted_at IS NULL
ORDER BY s.updated_at DESC
LIMIT 1;

-- name: GetSessionUsedSkills :many
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT DISTINCT
  (part->'input'->>'skillName')::text AS skill_name
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id,
  jsonb_array_elements(
    CASE WHEN jsonb_typeof(m.content->'content') = 'array'
         THEN m.content->'content'
         ELSE '[]'::jsonb
    END
  ) AS part
WHERE m.role = 'assistant'
  AND part->>'type' = 'tool-call'
  AND part->>'toolName' = 'use_skill'
  AND part->'input'->>'skillName' IS NOT NULL
  AND part->'input'->>'skillName' != ''
ORDER BY skill_name;
