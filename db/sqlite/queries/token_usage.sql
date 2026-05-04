-- name: GetTokenUsageByDayAndType :many
SELECT
  COALESCE(
    CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
    'chat'
  ) AS session_type,
  date(datetime(m.created_at)) AS day,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER)), 0) AS input_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.outputTokens') AS INTEGER)), 0) AS output_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheReadTokens') AS INTEGER)), 0) AS cache_read_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.outputTokenDetails.reasoningTokens') AS INTEGER)), 0) AS reasoning_tokens
FROM bot_history_messages m
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN bot_sessions ps ON ps.id = s.parent_session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND json_valid(m.usage)
  AND datetime(m.created_at) >= datetime(sqlc.arg(from_time))
  AND datetime(m.created_at) < datetime(sqlc.arg(to_time))
  AND (sqlc.narg(model_id) IS NULL OR m.model_id = sqlc.narg(model_id))
GROUP BY session_type, day
ORDER BY day, session_type;

-- name: GetTokenUsageByModel :many
SELECT
  m.model_id,
  COALESCE(mo.model_id, 'unknown') AS model_slug,
  COALESCE(mo.name, 'Unknown') AS model_name,
  COALESCE(lp.name, 'Unknown') AS provider_name,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER)), 0) AS input_tokens,
  COALESCE(SUM(CAST(json_extract(m.usage, '$.outputTokens') AS INTEGER)), 0) AS output_tokens
FROM bot_history_messages m
LEFT JOIN models mo ON mo.id = m.model_id
LEFT JOIN providers lp ON lp.id = mo.provider_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND json_valid(m.usage)
  AND datetime(m.created_at) >= datetime(sqlc.arg(from_time))
  AND datetime(m.created_at) < datetime(sqlc.arg(to_time))
GROUP BY m.model_id, mo.model_id, mo.name, lp.name
ORDER BY input_tokens DESC;

-- name: ListTokenUsageRecords :many
SELECT
  m.id,
  m.created_at,
  m.session_id,
  COALESCE(
    CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
    'chat'
  ) AS session_type,
  m.model_id,
  COALESCE(mo.model_id, 'unknown') AS model_slug,
  COALESCE(mo.name, 'Unknown') AS model_name,
  COALESCE(lp.name, 'Unknown') AS provider_name,
  COALESCE(CAST(json_extract(m.usage, '$.inputTokens') AS INTEGER), 0) AS input_tokens,
  COALESCE(CAST(json_extract(m.usage, '$.outputTokens') AS INTEGER), 0) AS output_tokens,
  COALESCE(CAST(json_extract(m.usage, '$.inputTokenDetails.cacheReadTokens') AS INTEGER), 0) AS cache_read_tokens,
  COALESCE(CAST(json_extract(m.usage, '$.outputTokenDetails.reasoningTokens') AS INTEGER), 0) AS reasoning_tokens
FROM bot_history_messages m
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN bot_sessions ps ON ps.id = s.parent_session_id
LEFT JOIN models mo ON mo.id = m.model_id
LEFT JOIN providers lp ON lp.id = mo.provider_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND json_valid(m.usage)
  AND datetime(m.created_at) >= datetime(sqlc.arg(from_time))
  AND datetime(m.created_at) < datetime(sqlc.arg(to_time))
  AND (sqlc.narg(model_id) IS NULL OR m.model_id = sqlc.narg(model_id))
  AND (
    sqlc.narg(session_type) IS NULL
    OR COALESCE(
      CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
      'chat'
    ) = sqlc.narg(session_type)
  )
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountTokenUsageRecords :one
SELECT COUNT(*) AS total
FROM bot_history_messages m
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN bot_sessions ps ON ps.id = s.parent_session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND json_valid(m.usage)
  AND datetime(m.created_at) >= datetime(sqlc.arg(from_time))
  AND datetime(m.created_at) < datetime(sqlc.arg(to_time))
  AND (sqlc.narg(model_id) IS NULL OR m.model_id = sqlc.narg(model_id))
  AND (
    sqlc.narg(session_type) IS NULL
    OR COALESCE(
      CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
      'chat'
    ) = sqlc.narg(session_type)
  );
