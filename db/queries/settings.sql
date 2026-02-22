-- name: GetSettingsByBotID :one
SELECT
  bots.id AS bot_id,
  bots.max_context_load_time,
  bots.max_context_tokens,
  bots.max_inbox_items,
  bots.language,
  bots.allow_guest,
  chat_models.model_id AS chat_model_id,
  memory_models.model_id AS memory_model_id,
  embedding_models.model_id AS embedding_model_id,
  search_providers.id AS search_provider_id
FROM bots
LEFT JOIN models AS chat_models ON chat_models.id = bots.chat_model_id
LEFT JOIN models AS memory_models ON memory_models.id = bots.memory_model_id
LEFT JOIN models AS embedding_models ON embedding_models.id = bots.embedding_model_id
LEFT JOIN search_providers ON search_providers.id = bots.search_provider_id
WHERE bots.id = $1;

-- name: UpsertBotSettings :one
WITH updated AS (
  UPDATE bots
  SET max_context_load_time = sqlc.arg(max_context_load_time),
      max_context_tokens = sqlc.arg(max_context_tokens),
      max_inbox_items = sqlc.arg(max_inbox_items),
      language = sqlc.arg(language),
      allow_guest = sqlc.arg(allow_guest),
      chat_model_id = COALESCE(sqlc.narg(chat_model_id)::uuid, bots.chat_model_id),
      memory_model_id = COALESCE(sqlc.narg(memory_model_id)::uuid, bots.memory_model_id),
      embedding_model_id = COALESCE(sqlc.narg(embedding_model_id)::uuid, bots.embedding_model_id),
      search_provider_id = COALESCE(sqlc.narg(search_provider_id)::uuid, bots.search_provider_id),
      updated_at = now()
  WHERE bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.max_context_load_time, bots.max_context_tokens, bots.max_inbox_items, bots.language, bots.allow_guest, bots.chat_model_id, bots.memory_model_id, bots.embedding_model_id, bots.search_provider_id
)
SELECT
  updated.id AS bot_id,
  updated.max_context_load_time,
  updated.max_context_tokens,
  updated.max_inbox_items,
  updated.language,
  updated.allow_guest,
  chat_models.model_id AS chat_model_id,
  memory_models.model_id AS memory_model_id,
  embedding_models.model_id AS embedding_model_id,
  search_providers.id AS search_provider_id
FROM updated
LEFT JOIN models AS chat_models ON chat_models.id = updated.chat_model_id
LEFT JOIN models AS memory_models ON memory_models.id = updated.memory_model_id
LEFT JOIN models AS embedding_models ON embedding_models.id = updated.embedding_model_id
LEFT JOIN search_providers ON search_providers.id = updated.search_provider_id;

-- name: DeleteSettingsByBotID :exec
UPDATE bots
SET max_context_load_time = 1440,
    max_context_tokens = 0,
    max_inbox_items = 50,
    language = 'auto',
    allow_guest = false,
    chat_model_id = NULL,
    memory_model_id = NULL,
    embedding_model_id = NULL,
    search_provider_id = NULL,
    updated_at = now()
WHERE id = $1;
