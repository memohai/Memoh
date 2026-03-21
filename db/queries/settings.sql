-- name: GetSettingsByBotID :one
SELECT
  bots.id AS bot_id,
  bots.max_context_load_time,
  bots.max_context_tokens,
  bots.language,
  bots.reasoning_enabled,
  bots.reasoning_effort,
  bots.heartbeat_enabled,
  bots.heartbeat_interval,
  bots.heartbeat_prompt,
  chat_models.id AS chat_model_id,
  heartbeat_models.id AS heartbeat_model_id,
  title_models.id AS title_model_id,
  search_providers.id AS search_provider_id,
  memory_providers.id AS memory_provider_id,
  tts_models.id AS tts_model_id,
  browser_contexts.id AS browser_context_id
FROM bots
LEFT JOIN models AS chat_models ON chat_models.id = bots.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = bots.heartbeat_model_id
LEFT JOIN models AS title_models ON title_models.id = bots.title_model_id
LEFT JOIN search_providers ON search_providers.id = bots.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = bots.memory_provider_id
LEFT JOIN tts_models ON tts_models.id = bots.tts_model_id
LEFT JOIN browser_contexts ON browser_contexts.id = bots.browser_context_id
WHERE bots.id = $1;

-- name: UpsertBotSettings :one
WITH updated AS (
  UPDATE bots
  SET max_context_load_time = sqlc.arg(max_context_load_time),
      max_context_tokens = sqlc.arg(max_context_tokens),
      language = sqlc.arg(language),
      reasoning_enabled = sqlc.arg(reasoning_enabled),
      reasoning_effort = sqlc.arg(reasoning_effort),
      heartbeat_enabled = sqlc.arg(heartbeat_enabled),
      heartbeat_interval = sqlc.arg(heartbeat_interval),
      heartbeat_prompt = sqlc.arg(heartbeat_prompt),
      chat_model_id = COALESCE(sqlc.narg(chat_model_id)::uuid, bots.chat_model_id),
      heartbeat_model_id = COALESCE(sqlc.narg(heartbeat_model_id)::uuid, bots.heartbeat_model_id),
      title_model_id = COALESCE(sqlc.narg(title_model_id)::uuid, bots.title_model_id),
      search_provider_id = COALESCE(sqlc.narg(search_provider_id)::uuid, bots.search_provider_id),
      memory_provider_id = COALESCE(sqlc.narg(memory_provider_id)::uuid, bots.memory_provider_id),
      tts_model_id = COALESCE(sqlc.narg(tts_model_id)::uuid, bots.tts_model_id),
      browser_context_id = COALESCE(sqlc.narg(browser_context_id)::uuid, bots.browser_context_id),
      updated_at = now()
  WHERE bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.max_context_load_time, bots.max_context_tokens, bots.language, bots.reasoning_enabled, bots.reasoning_effort, bots.heartbeat_enabled, bots.heartbeat_interval, bots.heartbeat_prompt, bots.chat_model_id, bots.heartbeat_model_id, bots.title_model_id, bots.search_provider_id, bots.memory_provider_id, bots.tts_model_id, bots.browser_context_id
)
SELECT
  updated.id AS bot_id,
  updated.max_context_load_time,
  updated.max_context_tokens,
  updated.language,
  updated.reasoning_enabled,
  updated.reasoning_effort,
  updated.heartbeat_enabled,
  updated.heartbeat_interval,
  updated.heartbeat_prompt,
  chat_models.id AS chat_model_id,
  heartbeat_models.id AS heartbeat_model_id,
  title_models.id AS title_model_id,
  search_providers.id AS search_provider_id,
  memory_providers.id AS memory_provider_id,
  tts_models.id AS tts_model_id,
  browser_contexts.id AS browser_context_id
FROM updated
LEFT JOIN models AS chat_models ON chat_models.id = updated.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = updated.heartbeat_model_id
LEFT JOIN models AS title_models ON title_models.id = updated.title_model_id
LEFT JOIN search_providers ON search_providers.id = updated.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = updated.memory_provider_id
LEFT JOIN tts_models ON tts_models.id = updated.tts_model_id
LEFT JOIN browser_contexts ON browser_contexts.id = updated.browser_context_id;

-- name: DeleteSettingsByBotID :exec
UPDATE bots
SET max_context_load_time = 1440,
    max_context_tokens = 0,
    language = 'auto',
    reasoning_enabled = false,
    reasoning_effort = 'medium',
    heartbeat_enabled = false,
    heartbeat_interval = 30,
    heartbeat_prompt = '',
    chat_model_id = NULL,
    heartbeat_model_id = NULL,
    title_model_id = NULL,
    search_provider_id = NULL,
    memory_provider_id = NULL,
    tts_model_id = NULL,
    browser_context_id = NULL,
    updated_at = now()
WHERE id = $1;
