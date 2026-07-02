-- name: CreateChatRoute :one
INSERT INTO bot_channel_routes (
  id, bot_id, channel_type, channel_config_id, external_conversation_id, external_thread_id,
  conversation_type, default_reply_target, metadata
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(platform),
  sqlc.narg(channel_config_id),
  sqlc.arg(conversation_id),
  sqlc.narg(thread_id),
  sqlc.narg(conversation_type),
  sqlc.narg(reply_target),
  sqlc.arg(metadata)
)
RETURNING
  id,
  bot_id AS chat_id,
  bot_id,
  channel_type AS platform,
  channel_config_id,
  external_conversation_id AS conversation_id,
  external_thread_id AS thread_id,
  conversation_type,
  default_reply_target AS reply_target,
  active_session_id,
  metadata,
  created_at,
  updated_at;

-- name: FindChatRoute :one
SELECT
  id,
  bot_id AS chat_id,
  bot_id,
  channel_type AS platform,
  channel_config_id,
  external_conversation_id AS conversation_id,
  external_thread_id AS thread_id,
  conversation_type,
  default_reply_target AS reply_target,
  active_session_id,
  metadata,
  created_at,
  updated_at
FROM bot_channel_routes
WHERE bot_id = sqlc.arg(bot_id)
  AND channel_type = sqlc.arg(platform)
  AND external_conversation_id = sqlc.arg(conversation_id)
  AND COALESCE(external_thread_id, '') = COALESCE(sqlc.narg(thread_id), '')
LIMIT 1;

-- name: GetChatRouteByID :one
SELECT
  id,
  bot_id AS chat_id,
  bot_id,
  channel_type AS platform,
  channel_config_id,
  external_conversation_id AS conversation_id,
  external_thread_id AS thread_id,
  conversation_type,
  default_reply_target AS reply_target,
  active_session_id,
  metadata,
  created_at,
  updated_at
FROM bot_channel_routes
WHERE id = sqlc.arg(id);

-- name: ListChatRoutes :many
SELECT
  id,
  bot_id AS chat_id,
  bot_id,
  channel_type AS platform,
  channel_config_id,
  external_conversation_id AS conversation_id,
  external_thread_id AS thread_id,
  conversation_type,
  default_reply_target AS reply_target,
  active_session_id,
  metadata,
  created_at,
  updated_at
FROM bot_channel_routes
WHERE bot_id = sqlc.arg(chat_id)
ORDER BY created_at ASC;

-- name: UpdateChatRouteReplyTarget :exec
UPDATE bot_channel_routes
SET default_reply_target = sqlc.arg(reply_target), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: UpdateChatRouteMetadata :exec
UPDATE bot_channel_routes
SET metadata = sqlc.arg(metadata), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: SetRouteActiveSession :exec
UPDATE bot_channel_routes
SET active_session_id = sqlc.narg(active_session_id), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteChatRoute :exec
DELETE FROM bot_channel_routes WHERE id = sqlc.arg(id);
