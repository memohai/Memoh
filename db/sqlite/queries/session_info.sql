-- name: CountMessagesBySession :one
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT COUNT(*) AS message_count
FROM bot_history_messages m
WHERE m.turn_id IN (SELECT id FROM visible_turns);

-- name: GetLatestAssistantUsage :one
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
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
  COALESCE(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER), 0) AS input_tokens
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
WHERE m.role = 'assistant'
  AND m.usage IS NOT NULL
  AND json_valid(m.usage)
ORDER BY vt.depth ASC, COALESCE(m.turn_message_seq, 9223372036854775807) DESC, m.created_at DESC
LIMIT 1;

-- name: GetSessionCacheStats :one
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER)), 0) AS total_input_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheReadTokens') AS INTEGER)), 0) AS cache_read_tokens
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
WHERE m.usage IS NOT NULL
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
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
  COALESCE(
    json_extract(j.value, '$.input.skillName'),
    json_extract(j.value, '$.input.skill_name'),
    json_extract(j.value, '$.input.name')
  ) AS skill_name
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id,
  json_each(
    CASE WHEN json_valid(m.content) AND json_type(m.content, '$.content') = 'array'
         THEN json_extract(m.content, '$.content')
         WHEN json_valid(m.content) AND json_type(m.content) = 'array'
         THEN m.content
         ELSE json('[]')
    END
  ) AS j
WHERE m.role = 'assistant'
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
