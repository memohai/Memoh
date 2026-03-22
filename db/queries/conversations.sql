-- name: CreateChat :one
SELECT
  b.id AS id,
  b.id AS bot_id,
  (COALESCE(NULLIF(sqlc.arg(kind)::text, ''), 'direct'))::text AS kind,
  CASE WHEN sqlc.arg(kind) = 'thread' THEN sqlc.arg(parent_chat_id)::uuid ELSE NULL::uuid END AS parent_chat_id,
  COALESCE(NULLIF(sqlc.arg(title)::text, ''), b.display_name) AS title,
  COALESCE(sqlc.arg(created_by_user_id)::uuid, b.owner_user_id) AS created_by_user_id,
  COALESCE(sqlc.arg(metadata)::jsonb, b.metadata) AS metadata,
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
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = $1;

-- name: ListChatsByBotAndUser :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
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
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at,
  'participant'::text AS access_mode,
  (CASE
    WHEN b.owner_user_id = sqlc.arg(user_id) THEN 'owner'
    ELSE ''::text
  END)::text AS participant_role,
  NULL::timestamptz AS last_observed_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(bot_id)
  AND b.owner_user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: GetChatReadAccessByUser :one
SELECT
  'participant'::text AS access_mode,
  'owner'::text AS participant_role,
  NULL::timestamptz AS last_observed_at
FROM bots b
WHERE b.id = sqlc.arg(chat_id)
  AND b.owner_user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListThreadsByParent :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = $1
ORDER BY b.created_at DESC;

-- name: UpdateChatTitle :one
WITH updated AS (
  UPDATE bots
  SET display_name = sqlc.arg(title),
      updated_at = now()
  WHERE bots.id = sqlc.arg(bot_id)
  RETURNING *
)
SELECT
  updated.id AS id,
  updated.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  updated.display_name AS title,
  updated.owner_user_id AS created_by_user_id,
  updated.metadata,
  chat_models.model_id AS model_id,
  updated.created_at,
  updated.updated_at
FROM updated
LEFT JOIN models chat_models ON chat_models.id = updated.chat_model_id;

-- name: TouchChat :exec
UPDATE bots
SET updated_at = now()
WHERE id = sqlc.arg(chat_id);

-- name: DeleteChat :exec
WITH deleted_messages AS (
  DELETE FROM bot_history_messages
  WHERE bot_id = sqlc.arg(chat_id)
),
deleted_sessions AS (
  DELETE FROM bot_sessions
  WHERE bot_id = sqlc.arg(chat_id)
)
DELETE FROM bot_channel_routes bcr
WHERE bcr.bot_id = sqlc.arg(chat_id);

-- name: GetChatParticipant :one
SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner'::text AS role, b.created_at AS joined_at
FROM bots b
WHERE b.id = sqlc.arg(chat_id) AND b.owner_user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListChatParticipants :many
SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner'::text AS role, b.created_at AS joined_at
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

-- chat_settings

-- name: UpsertChatSettings :one
WITH
updated AS (
  UPDATE bots
  SET chat_model_id = COALESCE(sqlc.narg(chat_model_id)::uuid, bots.chat_model_id),
      updated_at = now()
  WHERE bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.chat_model_id, bots.updated_at
)
SELECT
  updated.id AS chat_id,
  chat_models.id AS model_id,
  updated.updated_at
FROM updated
LEFT JOIN models chat_models ON chat_models.id = updated.chat_model_id;

-- name: GetChatSettings :one
SELECT
  b.id AS chat_id,
  chat_models.id AS model_id,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = $1;
