-- name: GetSettingsByBotID :one
SELECT
  bots.id AS bot_id,
  bots.language,
  bots.reasoning_enabled,
  bots.reasoning_effort,
  bots.heartbeat_enabled,
  bots.heartbeat_interval,
  bots.heartbeat_prompt,
  bots.compaction_enabled,
  bots.compaction_threshold,
  bots.compaction_ratio,
  chat_models.id AS chat_model_id,
  heartbeat_models.id AS heartbeat_model_id,
  compaction_models.id AS compaction_model_id,
  title_models.id AS title_model_id,
  search_providers.id AS search_provider_id,
  memory_providers.id AS memory_provider_id,
  tts_models.id AS tts_model_id,
  browser_contexts.id AS browser_context_id
FROM bots
LEFT JOIN models AS chat_models ON chat_models.id = bots.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = bots.heartbeat_model_id
LEFT JOIN models AS compaction_models ON compaction_models.id = bots.compaction_model_id
LEFT JOIN models AS title_models ON title_models.id = bots.title_model_id
LEFT JOIN search_providers ON search_providers.id = bots.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = bots.memory_provider_id
LEFT JOIN tts_models ON tts_models.id = bots.tts_model_id
LEFT JOIN browser_contexts ON browser_contexts.id = bots.browser_context_id
WHERE bots.id = $1;

-- name: UpsertBotSettings :one
WITH updated AS (
  UPDATE bots
  SET language = sqlc.arg(language),
      reasoning_enabled = sqlc.arg(reasoning_enabled),
      reasoning_effort = sqlc.arg(reasoning_effort),
      heartbeat_enabled = sqlc.arg(heartbeat_enabled),
      heartbeat_interval = sqlc.arg(heartbeat_interval),
      heartbeat_prompt = sqlc.arg(heartbeat_prompt),
      compaction_enabled = sqlc.arg(compaction_enabled),
      compaction_threshold = sqlc.arg(compaction_threshold),
      compaction_ratio = sqlc.arg(compaction_ratio),
      chat_model_id = COALESCE(sqlc.narg(chat_model_id)::uuid, bots.chat_model_id),
      heartbeat_model_id = COALESCE(sqlc.narg(heartbeat_model_id)::uuid, bots.heartbeat_model_id),
      compaction_model_id = COALESCE(sqlc.narg(compaction_model_id)::uuid, bots.compaction_model_id),
      title_model_id = COALESCE(sqlc.narg(title_model_id)::uuid, bots.title_model_id),
      search_provider_id = COALESCE(sqlc.narg(search_provider_id)::uuid, bots.search_provider_id),
      memory_provider_id = COALESCE(sqlc.narg(memory_provider_id)::uuid, bots.memory_provider_id),
      tts_model_id = COALESCE(sqlc.narg(tts_model_id)::uuid, bots.tts_model_id),
      browser_context_id = COALESCE(sqlc.narg(browser_context_id)::uuid, bots.browser_context_id),
      updated_at = now()
  WHERE bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.language, bots.reasoning_enabled, bots.reasoning_effort, bots.heartbeat_enabled, bots.heartbeat_interval, bots.heartbeat_prompt, bots.compaction_enabled, bots.compaction_threshold, bots.compaction_ratio, bots.chat_model_id, bots.heartbeat_model_id, bots.compaction_model_id, bots.title_model_id, bots.search_provider_id, bots.memory_provider_id, bots.tts_model_id, bots.browser_context_id
)
SELECT
  updated.id AS bot_id,
  updated.language,
  updated.reasoning_enabled,
  updated.reasoning_effort,
  updated.heartbeat_enabled,
  updated.heartbeat_interval,
  updated.heartbeat_prompt,
  updated.compaction_enabled,
  updated.compaction_threshold,
  updated.compaction_ratio,
  chat_models.id AS chat_model_id,
  heartbeat_models.id AS heartbeat_model_id,
  compaction_models.id AS compaction_model_id,
  title_models.id AS title_model_id,
  search_providers.id AS search_provider_id,
  memory_providers.id AS memory_provider_id,
  tts_models.id AS tts_model_id,
  browser_contexts.id AS browser_context_id
FROM updated
LEFT JOIN models AS chat_models ON chat_models.id = updated.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = updated.heartbeat_model_id
LEFT JOIN models AS compaction_models ON compaction_models.id = updated.compaction_model_id
LEFT JOIN models AS title_models ON title_models.id = updated.title_model_id
LEFT JOIN search_providers ON search_providers.id = updated.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = updated.memory_provider_id
LEFT JOIN tts_models ON tts_models.id = updated.tts_model_id
LEFT JOIN browser_contexts ON browser_contexts.id = updated.browser_context_id;

-- name: DeleteSettingsByBotID :exec
UPDATE bots
SET language = 'auto',
    reasoning_enabled = false,
    reasoning_effort = 'medium',
    heartbeat_enabled = false,
    heartbeat_interval = 30,
    heartbeat_prompt = '',
    compaction_enabled = false,
    compaction_threshold = 100000,
    compaction_ratio = 80,
    chat_model_id = NULL,
    heartbeat_model_id = NULL,
    compaction_model_id = NULL,
    title_model_id = NULL,
    search_provider_id = NULL,
    memory_provider_id = NULL,
    tts_model_id = NULL,
    browser_context_id = NULL,
    updated_at = now()
WHERE id = $1;
