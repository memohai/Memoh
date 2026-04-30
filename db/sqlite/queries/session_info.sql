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
  AND json_valid(m.usage)
ORDER BY m.created_at DESC
LIMIT 1;

-- name: GetSessionCacheStats :one
SELECT
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER)), 0) AS total_input_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheReadTokens') AS INTEGER)), 0) AS cache_read_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheWriteTokens') AS INTEGER)), 0) AS cache_write_tokens
FROM bot_history_messages m
WHERE m.session_id = sqlc.arg(session_id)
  AND m.usage IS NOT NULL
  AND json_valid(m.usage);

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
  COALESCE(
    json_extract(j.value, '$.input.skillName'),
    json_extract(j.value, '$.input.skill_name'),
    json_extract(j.value, '$.input.name')
  ) AS skill_name
FROM bot_history_messages m,
  json_each(
    CASE WHEN json_valid(m.content) AND json_type(m.content, '$.content') = 'array'
         THEN json_extract(m.content, '$.content')
         WHEN json_valid(m.content) AND json_type(m.content) = 'array'
         THEN m.content
         ELSE json('[]')
    END
  ) AS j
WHERE m.session_id = sqlc.arg(session_id)
  AND m.role = 'assistant'
  AND json_extract(j.value, '$.type') = 'tool-call'
  AND COALESCE(json_extract(j.value, '$.toolName'), json_extract(j.value, '$.tool_name')) = 'use_skill'
  AND COALESCE(
    json_extract(j.value, '$.input.skillName'),
    json_extract(j.value, '$.input.skill_name'),
    json_extract(j.value, '$.input.name')
  ) IS NOT NULL
  AND COALESCE(
    json_extract(j.value, '$.input.skillName'),
    json_extract(j.value, '$.input.skill_name'),
    json_extract(j.value, '$.input.name')
  ) != ''
ORDER BY skill_name;
