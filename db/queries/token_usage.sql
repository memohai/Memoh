-- name: GetTokenUsageByDayAndType :many
WITH usage_rows AS (
  SELECT
    m.session_id,
    m.model_id,
    m.created_at,
    COALESCE((m.usage->>'inputTokens')::bigint, 0)::bigint AS input_tokens,
    COALESCE((m.usage->>'outputTokens')::bigint, 0)::bigint AS output_tokens,
    COALESCE((m.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint, 0)::bigint AS cache_read_tokens,
    COALESCE((m.usage->'inputTokenDetails'->>'cacheWriteTokens')::bigint, 0)::bigint AS cache_write_tokens,
    COALESCE((m.usage->'outputTokenDetails'->>'reasoningTokens')::bigint, 0)::bigint AS reasoning_tokens
  FROM bot_history_messages m
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND m.usage IS NOT NULL
    AND m.created_at >= sqlc.arg(from_time)
    AND m.created_at < sqlc.arg(to_time)
    AND (sqlc.narg(model_id)::uuid IS NULL OR m.model_id = sqlc.narg(model_id)::uuid)
), non_negative_usage_rows AS (
  -- Exclude expired-token adjustment rows, which are stored as negative deltas.
  SELECT *
  FROM usage_rows
  WHERE input_tokens >= 0
    AND output_tokens >= 0
    AND cache_read_tokens >= 0
    AND cache_write_tokens >= 0
    AND reasoning_tokens >= 0
)
SELECT
  COALESCE(
    CASE WHEN s.type = 'subagent' THEN COALESCE(ps.type, 'chat') ELSE s.type END,
    'chat'
  )::text AS session_type,
  date_trunc('day', m.created_at)::date AS day,
  COALESCE(SUM(m.input_tokens), 0)::bigint AS input_tokens,
  COALESCE(SUM(m.output_tokens), 0)::bigint AS output_tokens,
  COALESCE(SUM(m.cache_read_tokens), 0)::bigint AS cache_read_tokens,
  COALESCE(SUM(m.cache_write_tokens), 0)::bigint AS cache_write_tokens,
  COALESCE(SUM(m.reasoning_tokens), 0)::bigint AS reasoning_tokens
FROM non_negative_usage_rows m
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN bot_sessions ps ON ps.id = s.parent_session_id
GROUP BY session_type, day
ORDER BY day, session_type;

-- name: GetTokenUsageByModel :many
WITH usage_rows AS (
  SELECT
    m.model_id,
    COALESCE((m.usage->>'inputTokens')::bigint, 0)::bigint AS input_tokens,
    COALESCE((m.usage->>'outputTokens')::bigint, 0)::bigint AS output_tokens,
    COALESCE((m.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint, 0)::bigint AS cache_read_tokens,
    COALESCE((m.usage->'inputTokenDetails'->>'cacheWriteTokens')::bigint, 0)::bigint AS cache_write_tokens,
    COALESCE((m.usage->'outputTokenDetails'->>'reasoningTokens')::bigint, 0)::bigint AS reasoning_tokens
  FROM bot_history_messages m
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND m.usage IS NOT NULL
    AND m.created_at >= sqlc.arg(from_time)
    AND m.created_at < sqlc.arg(to_time)
), non_negative_usage_rows AS (
  -- Exclude expired-token adjustment rows, which are stored as negative deltas.
  SELECT *
  FROM usage_rows
  WHERE input_tokens >= 0
    AND output_tokens >= 0
    AND cache_read_tokens >= 0
    AND cache_write_tokens >= 0
    AND reasoning_tokens >= 0
)
SELECT
  m.model_id,
  COALESCE(mo.model_id, 'unknown') AS model_slug,
  COALESCE(mo.name, 'Unknown') AS model_name,
  COALESCE(lp.name, 'Unknown') AS provider_name,
  COALESCE(SUM(m.input_tokens), 0)::bigint AS input_tokens,
  COALESCE(SUM(m.output_tokens), 0)::bigint AS output_tokens
FROM non_negative_usage_rows m
LEFT JOIN models mo ON mo.id = m.model_id
LEFT JOIN providers lp ON lp.id = mo.provider_id
GROUP BY m.model_id, mo.model_id, mo.name, lp.name
ORDER BY input_tokens DESC;
