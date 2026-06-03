-- name: CreateBot :one
INSERT INTO bots (owner_user_id, name, display_name, avatar_url, timezone, is_active, metadata, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: GetBotByID :one
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id, metadata, created_at, updated_at
FROM bots
WHERE id = $1;

-- name: GetBotByName :one
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id, metadata, created_at, updated_at
FROM bots
WHERE name = $1;

-- name: ListBotsByOwner :many
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at
FROM bots
WHERE owner_user_id = $1
ORDER BY created_at DESC;

-- name: ListAccessibleBots :many
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at
FROM bots b
WHERE b.owner_user_id = $1
   OR EXISTS (
     SELECT 1 FROM bot_user_grants g
     WHERE g.bot_id = b.id
       AND (
         g.subject_type = 'everyone'
         OR (g.subject_type = 'user' AND g.user_id = $1)
       )
   )
ORDER BY b.created_at DESC;

-- name: UpdateBotProfile :one
UPDATE bots
SET name = $2,
    display_name = $3,
    avatar_url = $4,
    timezone = $5,
    is_active = $6,
    metadata = $7,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: UpdateBotOwner :one
UPDATE bots
SET owner_user_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

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
