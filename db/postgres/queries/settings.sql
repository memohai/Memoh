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
  bots.timezone,
  chat_models.id AS chat_model_id,
  heartbeat_models.id AS heartbeat_model_id,
  compaction_models.id AS compaction_model_id,
  title_models.id AS title_model_id,
  search_providers.id AS search_provider_id,
  memory_providers.id AS memory_provider_id,
  image_models.id AS image_model_id,
  tts_models.id AS tts_model_id,
  transcription_models.id AS transcription_model_id,
  browser_contexts.id AS browser_context_id,
  bots.persist_full_tool_results,
  bots.show_tool_calls_in_im,
  bots.tool_approval_config
FROM bots
LEFT JOIN models AS chat_models ON chat_models.id = bots.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = bots.heartbeat_model_id
LEFT JOIN models AS compaction_models ON compaction_models.id = bots.compaction_model_id
LEFT JOIN models AS title_models ON title_models.id = bots.title_model_id
LEFT JOIN models AS image_models ON image_models.id = bots.image_model_id
LEFT JOIN search_providers ON search_providers.id = bots.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = bots.memory_provider_id
LEFT JOIN models AS tts_models ON tts_models.id = bots.tts_model_id
LEFT JOIN models AS transcription_models ON transcription_models.id = bots.transcription_model_id
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
      timezone = COALESCE(sqlc.narg(timezone)::text, bots.timezone),
      chat_model_id = COALESCE(sqlc.narg(chat_model_id)::uuid, bots.chat_model_id),
      heartbeat_model_id = COALESCE(sqlc.narg(heartbeat_model_id)::uuid, bots.heartbeat_model_id),
      compaction_model_id = COALESCE(sqlc.narg(compaction_model_id)::uuid, bots.compaction_model_id),
      title_model_id = COALESCE(sqlc.narg(title_model_id)::uuid, bots.title_model_id),
      search_provider_id = COALESCE(sqlc.narg(search_provider_id)::uuid, bots.search_provider_id),
      memory_provider_id = COALESCE(sqlc.narg(memory_provider_id)::uuid, bots.memory_provider_id),
      image_model_id = COALESCE(sqlc.narg(image_model_id)::uuid, bots.image_model_id),
      tts_model_id = COALESCE(sqlc.narg(tts_model_id)::uuid, bots.tts_model_id),
      transcription_model_id = COALESCE(sqlc.narg(transcription_model_id)::uuid, bots.transcription_model_id),
      browser_context_id = COALESCE(sqlc.narg(browser_context_id)::uuid, bots.browser_context_id),
      persist_full_tool_results = sqlc.arg(persist_full_tool_results),
      show_tool_calls_in_im = sqlc.arg(show_tool_calls_in_im),
      tool_approval_config = sqlc.arg(tool_approval_config),
      updated_at = now()
  WHERE bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.language, bots.reasoning_enabled, bots.reasoning_effort, bots.heartbeat_enabled, bots.heartbeat_interval, bots.heartbeat_prompt, bots.compaction_enabled, bots.compaction_threshold, bots.compaction_ratio, bots.timezone, bots.chat_model_id, bots.heartbeat_model_id, bots.compaction_model_id, bots.title_model_id, bots.image_model_id, bots.search_provider_id, bots.memory_provider_id, bots.tts_model_id, bots.transcription_model_id, bots.browser_context_id, bots.persist_full_tool_results, bots.show_tool_calls_in_im, bots.tool_approval_config
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
  updated.timezone,
  chat_models.id AS chat_model_id,
  heartbeat_models.id AS heartbeat_model_id,
  compaction_models.id AS compaction_model_id,
  title_models.id AS title_model_id,
  search_providers.id AS search_provider_id,
  memory_providers.id AS memory_provider_id,
  image_models.id AS image_model_id,
  tts_models.id AS tts_model_id,
  transcription_models.id AS transcription_model_id,
  browser_contexts.id AS browser_context_id,
  updated.persist_full_tool_results,
  updated.show_tool_calls_in_im,
  updated.tool_approval_config
FROM updated
LEFT JOIN models AS chat_models ON chat_models.id = updated.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = updated.heartbeat_model_id
LEFT JOIN models AS compaction_models ON compaction_models.id = updated.compaction_model_id
LEFT JOIN models AS title_models ON title_models.id = updated.title_model_id
LEFT JOIN models AS image_models ON image_models.id = updated.image_model_id
LEFT JOIN search_providers ON search_providers.id = updated.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = updated.memory_provider_id
LEFT JOIN models AS tts_models ON tts_models.id = updated.tts_model_id
LEFT JOIN models AS transcription_models ON transcription_models.id = updated.transcription_model_id
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
    image_model_id = NULL,
    search_provider_id = NULL,
    memory_provider_id = NULL,
    tts_model_id = NULL,
    transcription_model_id = NULL,
    browser_context_id = NULL,
    persist_full_tool_results = false,
    show_tool_calls_in_im = false,
    tool_approval_config = '{"enabled":false,"write":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"edit":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"exec":{"require_approval":false,"bypass_commands":[],"force_review_commands":[]}}'::jsonb,
    updated_at = now()
WHERE id = $1;
