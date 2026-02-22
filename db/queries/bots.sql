-- name: CreateBot :one
INSERT INTO bots (owner_user_id, type, display_name, avatar_url, is_active, metadata, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, owner_user_id, type, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, max_inbox_items, language, allow_guest, chat_model_id, memory_model_id, embedding_model_id, search_provider_id, metadata, created_at, updated_at;

-- name: GetBotByID :one
SELECT id, owner_user_id, type, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, max_inbox_items, language, allow_guest, chat_model_id, memory_model_id, embedding_model_id, search_provider_id, metadata, created_at, updated_at
FROM bots
WHERE id = $1;

-- name: ListBotsByOwner :many
SELECT id, owner_user_id, type, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, max_inbox_items, language, allow_guest, chat_model_id, memory_model_id, embedding_model_id, search_provider_id, metadata, created_at, updated_at
FROM bots
WHERE owner_user_id = $1
ORDER BY created_at DESC;

-- name: ListBotsByMember :many
SELECT b.id, b.owner_user_id, b.type, b.display_name, b.avatar_url, b.is_active, b.status, b.max_context_load_time, b.max_context_tokens, b.max_inbox_items, b.language, b.allow_guest, b.chat_model_id, b.memory_model_id, b.embedding_model_id, b.search_provider_id, b.metadata, b.created_at, b.updated_at
FROM bots b
JOIN bot_members m ON m.bot_id = b.id
WHERE m.user_id = $1
ORDER BY b.created_at DESC;

-- name: UpdateBotProfile :one
UPDATE bots
SET display_name = $2,
    avatar_url = $3,
    is_active = $4,
    metadata = $5,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_user_id, type, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, max_inbox_items, language, allow_guest, chat_model_id, memory_model_id, embedding_model_id, search_provider_id, metadata, created_at, updated_at;

-- name: UpdateBotOwner :one
UPDATE bots
SET owner_user_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_user_id, type, display_name, avatar_url, is_active, status, max_context_load_time, max_context_tokens, max_inbox_items, language, allow_guest, chat_model_id, memory_model_id, embedding_model_id, search_provider_id, metadata, created_at, updated_at;

-- name: UpdateBotStatus :exec
UPDATE bots
SET status = $2,
    updated_at = now()
WHERE id = $1;

-- name: DeleteBotByID :exec
DELETE FROM bots WHERE id = $1;

-- name: UpsertBotMember :one
INSERT INTO bot_members (bot_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (bot_id, user_id) DO UPDATE SET
  role = EXCLUDED.role
RETURNING bot_id, user_id, role, created_at;

-- name: ListBotMembers :many
SELECT bot_id, user_id, role, created_at
FROM bot_members
WHERE bot_id = $1
ORDER BY created_at DESC;

-- name: GetBotMember :one
SELECT bot_id, user_id, role, created_at
FROM bot_members
WHERE bot_id = $1 AND user_id = $2
LIMIT 1;

-- name: DeleteBotMember :exec
DELETE FROM bot_members WHERE bot_id = $1 AND user_id = $2;
