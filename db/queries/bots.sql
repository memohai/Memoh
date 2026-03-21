-- name: CreateBot :one
INSERT INTO bots (owner_user_id, display_name, avatar_url, is_active, metadata, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, owner_user_id, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: GetBotByID :one
SELECT id, owner_user_id, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at
FROM bots
WHERE id = $1;

-- name: ListBotsByOwner :many
SELECT id, owner_user_id, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at
FROM bots
WHERE owner_user_id = $1
ORDER BY created_at DESC;

-- name: UpdateBotProfile :one
UPDATE bots
SET display_name = $2,
    avatar_url = $3,
    is_active = $4,
    metadata = $5,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_user_id, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: UpdateBotOwner :one
UPDATE bots
SET owner_user_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_user_id, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: UpdateBotStatus :exec
UPDATE bots
SET status = $2,
    updated_at = now()
WHERE id = $1;

-- name: DeleteBotByID :exec
DELETE FROM bots WHERE id = $1;

-- name: ListHeartbeatEnabledBots :many
SELECT id, owner_user_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt
FROM bots
WHERE heartbeat_enabled = true AND status = 'ready';
