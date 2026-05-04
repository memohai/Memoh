-- name: CreateChat :one
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct' AS kind,
  NULL AS parent_chat_id,
  COALESCE(NULLIF(sqlc.arg(title), ''), b.display_name) AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(bot_id)
LIMIT 1;

-- name: GetChatByID :one
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct' AS kind,
  NULL AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(id);

-- name: ListChatsByBotAndUser :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct' AS kind,
  NULL AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(bot_id)
  AND b.owner_user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: ListVisibleChatsByBotAndUser :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct' AS kind,
  NULL AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at,
  'participant' AS access_mode,
  CASE
    WHEN b.owner_user_id = sqlc.arg(user_id) THEN 'owner'
    ELSE ''
  END AS participant_role,
  NULL AS last_observed_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(bot_id)
  AND b.owner_user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: GetChatReadAccessByUser :one
SELECT
  'participant' AS access_mode,
  'owner' AS participant_role,
  NULL AS last_observed_at
FROM bots b
WHERE b.id = sqlc.arg(chat_id)
  AND b.owner_user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListThreadsByParent :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct' AS kind,
  NULL AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(id)
ORDER BY b.created_at DESC;

-- name: UpdateChatTitle :one
UPDATE bots
SET display_name = sqlc.arg(title),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(bot_id)
RETURNING
  id AS id,
  id AS bot_id,
  'direct' AS kind,
  NULL AS parent_chat_id,
  display_name AS title,
  owner_user_id AS created_by_user_id,
  metadata,
  chat_model_id AS model_id,
  created_at,
  updated_at;

-- name: TouchChat :exec
UPDATE bots
SET updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(chat_id);

-- name: DeleteChat :exec
DELETE FROM bot_history_messages WHERE bot_id = sqlc.arg(chat_id);

-- name: DeleteChatRoutes :exec
DELETE FROM bot_channel_routes WHERE bot_id = sqlc.arg(chat_id);

-- name: DeleteChatSessions :exec
DELETE FROM bot_sessions WHERE bot_id = sqlc.arg(chat_id);

-- name: GetChatParticipant :one
SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner' AS role, b.created_at AS joined_at
FROM bots b
WHERE b.id = sqlc.arg(chat_id) AND b.owner_user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListChatParticipants :many
SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner' AS role, b.created_at AS joined_at
FROM bots b
WHERE b.id = sqlc.arg(chat_id)
ORDER BY joined_at ASC;

-- name: RemoveChatParticipant :exec
SELECT 1
WHERE EXISTS (
  SELECT 1
  FROM bots b
  WHERE b.id = sqlc.arg(chat_id)
    AND b.owner_user_id = sqlc.arg(user_id)
);

-- name: UpsertChatSettings :one
UPDATE bots
SET chat_model_id = COALESCE(sqlc.narg(chat_model_id), bots.chat_model_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING
  id AS chat_id,
  chat_model_id AS model_id,
  updated_at;

-- name: GetChatSettings :one
SELECT
  b.id AS chat_id,
  chat_models.id AS model_id,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(id);
