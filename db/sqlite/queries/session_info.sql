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
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheReadTokens') AS INTEGER)), 0) AS cache_read_tokens
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
WITH requested_payloads AS (
  SELECT
    CASE WHEN json_valid(m.metadata) AND json_type(m.metadata, '$.model_requested_skills') = 'array'
         THEN json_extract(m.metadata, '$.model_requested_skills')
         ELSE json('[]')
    END AS skills_json
  FROM bot_history_messages m
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.role = 'user'
),
requested AS (
  SELECT DISTINCT
    json_extract(j.value, '$.name') AS skill_name,
    'requested:' ||
      COALESCE(NULLIF(json_extract(j.value, '$.source_kind'), ''), 'unknown') || ':' ||
      COALESCE(NULLIF(json_extract(j.value, '$.opaque_source_id'), ''), '') || ':' ||
      json_extract(j.value, '$.name') AS skill_identity
  FROM requested_payloads p,
    json_each(p.skills_json) AS j
  WHERE json_extract(j.value, '$.name') IS NOT NULL
    AND json_extract(j.value, '$.name') != ''
),
tool_payloads AS (
  SELECT
    CASE WHEN json_valid(m.content) AND json_type(m.content, '$.content') = 'array'
         THEN json_extract(m.content, '$.content')
         WHEN json_valid(m.content) AND json_type(m.content) = 'array'
         THEN m.content
         ELSE json('[]')
    END AS content_json
  FROM bot_history_messages m
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.role = 'assistant'
),
tool_used AS (
  SELECT DISTINCT
    COALESCE(
      json_extract(j.value, '$.input.skillName'),
      json_extract(j.value, '$.input.skill_name'),
      json_extract(j.value, '$.input.name')
    ) AS skill_name,
    'tool_call:' || COALESCE(
      json_extract(j.value, '$.input.skillName'),
      json_extract(j.value, '$.input.skill_name'),
      json_extract(j.value, '$.input.name')
    ) AS skill_identity
  FROM tool_payloads p,
    json_each(p.content_json) AS j
  WHERE json_extract(j.value, '$.type') = 'tool-call'
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
),
skill_rows AS (
  SELECT skill_identity, skill_name FROM requested
  UNION ALL
  SELECT skill_identity, skill_name FROM tool_used
),
deduped AS (
  SELECT DISTINCT skill_identity, skill_name FROM skill_rows
)
SELECT DISTINCT skill_name FROM deduped
ORDER BY skill_name;
