-- name: CreateBot :one
INSERT INTO bots (id, owner_user_id, type, name, display_name, avatar_url, timezone, is_active, metadata, status)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(owner_user_id),
  'personal',
  sqlc.arg(name),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(timezone),
  sqlc.arg(is_active),
  sqlc.arg(metadata),
  sqlc.arg(status)
)
RETURNING id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: GetBotByID :one
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id, metadata, created_at, updated_at
FROM bots
WHERE id = sqlc.arg(id);

-- name: GetBotByName :one
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id, metadata, created_at, updated_at
FROM bots
WHERE name = sqlc.arg(name);

-- name: ListBotsByOwner :many
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at
FROM bots
WHERE owner_user_id = sqlc.arg(owner_user_id)
ORDER BY created_at DESC;

-- name: ListAccessibleBots :many
SELECT id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at
FROM bots b
WHERE b.owner_user_id = sqlc.arg(user_id)
   OR EXISTS (
     SELECT 1 FROM bot_user_grants g
     WHERE g.bot_id = b.id
       AND (
         g.subject_type = 'everyone'
         OR (g.subject_type = 'user' AND g.user_id = sqlc.arg(user_id))
       )
   )
ORDER BY b.created_at DESC;

-- name: UpdateBotProfile :one
UPDATE bots
SET name = sqlc.arg(name),
    display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    timezone = sqlc.arg(timezone),
    is_active = sqlc.arg(is_active),
    metadata = sqlc.arg(metadata),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: UpdateBotOwner :one
UPDATE bots
SET owner_user_id = sqlc.arg(owner_user_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at;

-- name: UpdateBotStatus :exec
UPDATE bots
SET status = sqlc.arg(status),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteBotByID :exec
DELETE FROM bots WHERE id = sqlc.arg(id);

-- name: ListHeartbeatEnabledBots :many
SELECT id, owner_user_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt
FROM bots
WHERE heartbeat_enabled = true AND status = 'ready';
