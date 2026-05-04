-- name: GetTokenUsageByDayAndType :many
SELECT
  COALESCE(
    CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
    'chat'
  )::text AS session_type,
  date_trunc('day', m.created_at)::date AS day,
  COALESCE(SUM((m.usage->>'inputTokens')::bigint), 0)::bigint AS input_tokens,
  COALESCE(SUM((m.usage->>'outputTokens')::bigint), 0)::bigint AS output_tokens,
  COALESCE(SUM((m.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint), 0)::bigint AS cache_read_tokens,
  COALESCE(SUM((m.usage->'outputTokenDetails'->>'reasoningTokens')::bigint), 0)::bigint AS reasoning_tokens
FROM bot_history_messages m
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN bot_sessions ps ON ps.id = s.parent_session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND m.created_at >= sqlc.arg(from_time)
  AND m.created_at < sqlc.arg(to_time)
  AND (sqlc.narg(model_id)::uuid IS NULL OR m.model_id = sqlc.narg(model_id)::uuid)
GROUP BY session_type, day
ORDER BY day, session_type;

-- name: GetTokenUsageByModel :many
SELECT
  m.model_id,
  COALESCE(mo.model_id, 'unknown') AS model_slug,
  COALESCE(mo.name, 'Unknown') AS model_name,
  COALESCE(lp.name, 'Unknown') AS provider_name,
  COALESCE(SUM((m.usage->>'inputTokens')::bigint), 0)::bigint AS input_tokens,
  COALESCE(SUM((m.usage->>'outputTokens')::bigint), 0)::bigint AS output_tokens
FROM bot_history_messages m
LEFT JOIN models mo ON mo.id = m.model_id
LEFT JOIN providers lp ON lp.id = mo.provider_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND m.created_at >= sqlc.arg(from_time)
  AND m.created_at < sqlc.arg(to_time)
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
  )::text AS session_type,
  m.model_id,
  COALESCE(mo.model_id, 'unknown')::text AS model_slug,
  COALESCE(mo.name, 'Unknown')::text AS model_name,
  COALESCE(lp.name, 'Unknown')::text AS provider_name,
  COALESCE((m.usage->>'inputTokens')::bigint, 0)::bigint AS input_tokens,
  COALESCE((m.usage->>'outputTokens')::bigint, 0)::bigint AS output_tokens,
  COALESCE((m.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint, 0)::bigint AS cache_read_tokens,
  COALESCE((m.usage->'outputTokenDetails'->>'reasoningTokens')::bigint, 0)::bigint AS reasoning_tokens
FROM bot_history_messages m
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN bot_sessions ps ON ps.id = s.parent_session_id
LEFT JOIN models mo ON mo.id = m.model_id
LEFT JOIN providers lp ON lp.id = mo.provider_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND m.created_at >= sqlc.arg(from_time)
  AND m.created_at < sqlc.arg(to_time)
  AND (sqlc.narg(model_id)::uuid IS NULL OR m.model_id = sqlc.narg(model_id)::uuid)
  AND (
    sqlc.narg(session_type)::text IS NULL
    OR COALESCE(
      CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
      'chat'
    ) = sqlc.narg(session_type)::text
  )
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountTokenUsageRecords :one
SELECT COUNT(*)::bigint AS total
FROM bot_history_messages m
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN bot_sessions ps ON ps.id = s.parent_session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND m.created_at >= sqlc.arg(from_time)
  AND m.created_at < sqlc.arg(to_time)
  AND (sqlc.narg(model_id)::uuid IS NULL OR m.model_id = sqlc.narg(model_id)::uuid)
  AND (
    sqlc.narg(session_type)::text IS NULL
    OR COALESCE(
      CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
      'chat'
    ) = sqlc.narg(session_type)::text
  );
