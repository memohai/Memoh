-- name: CountMessagesBySession :one
SELECT COUNT(*) AS message_count
FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id);

-- name: GetLatestAssistantUsage :one
SELECT
  COALESCE(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER), 0) AS input_tokens
FROM bot_history_messages m
WHERE m.session_id = sqlc.arg(session_id)
  AND m.role = 'assistant'
  AND m.usage IS NOT NULL
ORDER BY m.created_at DESC
LIMIT 1;

-- name: GetSessionCacheStats :one
SELECT
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER)), 0) AS total_input_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheReadTokens') AS INTEGER)), 0) AS cache_read_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheWriteTokens') AS INTEGER)), 0) AS cache_write_tokens
FROM bot_history_messages m
WHERE m.session_id = sqlc.arg(session_id)
  AND m.usage IS NOT NULL;

-- name: GetLatestSessionIDByBot :one
SELECT s.id
FROM bot_sessions s
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.type = 'chat'
  AND s.deleted_at IS NULL
ORDER BY s.updated_at DESC
LIMIT 1;

-- name: GetSessionUsedSkills :many
SELECT DISTINCT
  json_extract(j.value, '$.input.skillName') AS skill_name
FROM bot_history_messages m,
  json_each(
    CASE WHEN json_type(json_extract(m.content, '$.content')) = 'array'
         THEN json_extract(m.content, '$.content')
         ELSE '[]'
    END
  ) AS j
WHERE m.session_id = sqlc.arg(session_id)
  AND m.role = 'assistant'
  AND json_extract(j.value, '$.type') = 'tool-call'
  AND json_extract(j.value, '$.toolName') = 'use_skill'
  AND json_extract(j.value, '$.input.skillName') IS NOT NULL
  AND json_extract(j.value, '$.input.skillName') != ''
ORDER BY skill_name;
