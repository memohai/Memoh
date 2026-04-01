-- name: CountMessagesBySession :one
SELECT COUNT(*)::bigint AS message_count
FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id);

-- name: GetLatestAssistantUsage :one
SELECT
  COALESCE((m.usage->>'inputTokens')::bigint, 0)::bigint AS input_tokens
FROM bot_history_messages m
WHERE m.session_id = sqlc.arg(session_id)
  AND m.role = 'assistant'
  AND m.usage IS NOT NULL
ORDER BY m.created_at DESC
LIMIT 1;

-- name: GetSessionCacheStats :one
SELECT
  COALESCE(SUM((m.usage->>'inputTokens')::bigint), 0)::bigint AS total_input_tokens,
  COALESCE(SUM((m.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint), 0)::bigint AS cache_read_tokens,
  COALESCE(SUM((m.usage->'inputTokenDetails'->>'cacheWriteTokens')::bigint), 0)::bigint AS cache_write_tokens
FROM bot_history_messages m
WHERE m.session_id = sqlc.arg(session_id)
  AND m.usage IS NOT NULL;

-- name: GetSessionUsedSkills :many
SELECT DISTINCT
  (part->'input'->>'skillName')::text AS skill_name
FROM bot_history_messages m,
  jsonb_array_elements(
    CASE WHEN jsonb_typeof(m.content->'content') = 'array'
         THEN m.content->'content'
         ELSE '[]'::jsonb
    END
  ) AS part
WHERE m.session_id = sqlc.arg(session_id)
  AND m.role = 'assistant'
  AND part->>'type' = 'tool-call'
  AND part->>'toolName' = 'use_skill'
  AND part->'input'->>'skillName' IS NOT NULL
  AND part->'input'->>'skillName' != ''
ORDER BY skill_name;
