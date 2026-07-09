-- name: CreateChatRoute :one
INSERT INTO bot_channel_routes (
  team_id, bot_id, channel_type, channel_config_id, external_conversation_id, external_thread_id, conversation_type, default_reply_target, metadata
)
SELECT
  b.team_id,
  b.id,
  sqlc.arg(platform),
  sqlc.narg(channel_config_id)::uuid,
  sqlc.arg(conversation_id),
  sqlc.narg(thread_id)::text,
  sqlc.narg(conversation_type)::text,
  sqlc.narg(reply_target)::text,
  sqlc.arg(metadata)
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
RETURNING
  id,
  sqlc.arg(chat_id)::uuid AS chat_id,
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
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
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
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

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
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(chat_id)
ORDER BY created_at ASC;

-- name: UpdateChatRouteReplyTarget :exec
UPDATE bot_channel_routes
SET default_reply_target = sqlc.arg(reply_target), updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: UpdateChatRouteMetadata :exec
UPDATE bot_channel_routes
SET metadata = sqlc.arg(metadata), updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: SetRouteActiveSession :exec
UPDATE bot_channel_routes
SET active_session_id = sqlc.narg(active_session_id)::uuid, updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: DeleteChatRoute :exec
DELETE FROM bot_channel_routes
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);
