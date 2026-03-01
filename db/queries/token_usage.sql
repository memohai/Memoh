-- name: GetMessageTokenUsageByDay :many
SELECT
  date_trunc('day', m.created_at)::date AS day,
  COALESCE(SUM((m.usage->>'inputTokens')::bigint), 0)::bigint AS input_tokens,
  COALESCE(SUM((m.usage->>'outputTokens')::bigint), 0)::bigint AS output_tokens,
  COALESCE(SUM((m.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint), 0)::bigint AS cache_read_tokens,
  COALESCE(SUM((m.usage->'inputTokenDetails'->>'cacheWriteTokens')::bigint), 0)::bigint AS cache_write_tokens,
  COALESCE(SUM((m.usage->'outputTokenDetails'->>'reasoningTokens')::bigint), 0)::bigint AS reasoning_tokens
FROM bot_history_messages m
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND m.created_at >= sqlc.arg(from_time)
  AND m.created_at < sqlc.arg(to_time)
  AND (sqlc.narg(model_id)::uuid IS NULL OR m.model_id = sqlc.narg(model_id)::uuid)
GROUP BY day
ORDER BY day;

-- name: GetHeartbeatTokenUsageByDay :many
SELECT
  date_trunc('day', h.started_at)::date AS day,
  COALESCE(SUM((h.usage->>'inputTokens')::bigint), 0)::bigint AS input_tokens,
  COALESCE(SUM((h.usage->>'outputTokens')::bigint), 0)::bigint AS output_tokens,
  COALESCE(SUM((h.usage->'inputTokenDetails'->>'cacheReadTokens')::bigint), 0)::bigint AS cache_read_tokens,
  COALESCE(SUM((h.usage->'inputTokenDetails'->>'cacheWriteTokens')::bigint), 0)::bigint AS cache_write_tokens,
  COALESCE(SUM((h.usage->'outputTokenDetails'->>'reasoningTokens')::bigint), 0)::bigint AS reasoning_tokens
FROM bot_heartbeat_logs h
WHERE h.bot_id = sqlc.arg(bot_id)
  AND h.usage IS NOT NULL
  AND h.started_at >= sqlc.arg(from_time)
  AND h.started_at < sqlc.arg(to_time)
  AND (sqlc.narg(model_id)::uuid IS NULL OR h.model_id = sqlc.narg(model_id)::uuid)
GROUP BY day
ORDER BY day;

-- name: GetMessageTokenUsageByModel :many
SELECT
  m.model_id,
  COALESCE(mo.model_id, 'unknown') AS model_slug,
  COALESCE(mo.name, 'Unknown') AS model_name,
  COALESCE(lp.name, 'Unknown') AS provider_name,
  COALESCE(SUM((m.usage->>'inputTokens')::bigint), 0)::bigint AS input_tokens,
  COALESCE(SUM((m.usage->>'outputTokens')::bigint), 0)::bigint AS output_tokens
FROM bot_history_messages m
LEFT JOIN models mo ON mo.id = m.model_id
LEFT JOIN llm_providers lp ON lp.id = mo.llm_provider_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.usage IS NOT NULL
  AND m.created_at >= sqlc.arg(from_time)
  AND m.created_at < sqlc.arg(to_time)
GROUP BY m.model_id, mo.model_id, mo.name, lp.name
ORDER BY input_tokens DESC;

-- name: GetHeartbeatTokenUsageByModel :many
SELECT
  h.model_id,
  COALESCE(mo.model_id, 'unknown') AS model_slug,
  COALESCE(mo.name, 'Unknown') AS model_name,
  COALESCE(lp.name, 'Unknown') AS provider_name,
  COALESCE(SUM((h.usage->>'inputTokens')::bigint), 0)::bigint AS input_tokens,
  COALESCE(SUM((h.usage->>'outputTokens')::bigint), 0)::bigint AS output_tokens
FROM bot_heartbeat_logs h
LEFT JOIN models mo ON mo.id = h.model_id
LEFT JOIN llm_providers lp ON lp.id = mo.llm_provider_id
WHERE h.bot_id = sqlc.arg(bot_id)
  AND h.usage IS NOT NULL
  AND h.started_at >= sqlc.arg(from_time)
  AND h.started_at < sqlc.arg(to_time)
GROUP BY h.model_id, mo.model_id, mo.name, lp.name
ORDER BY input_tokens DESC;
