-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  bot_id,
  route_id,
  sender_channel_identity_id,
  sender_account_user_id,
  channel_type,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  model_id
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(route_id)::uuid,
  sqlc.narg(sender_channel_identity_id)::uuid,
  sqlc.narg(sender_user_id)::uuid,
  sqlc.narg(platform)::text,
  sqlc.narg(external_message_id)::text,
  sqlc.narg(source_reply_to_message_id)::text,
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.narg(model_id)::uuid
)
RETURNING
  id,
  bot_id,
  route_id,
  sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  channel_type AS platform,
  source_message_id AS external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  created_at;

-- name: ListMessages :many
SELECT
  m.id,
  m.bot_id,
  m.route_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.channel_type AS platform,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at ASC
LIMIT 10000;

-- name: ListMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.route_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.channel_type AS platform,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.created_at ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.route_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.channel_type AS platform,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY m.created_at ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id,
  m.bot_id,
  m.route_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.channel_type AS platform,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id,
  m.bot_id,
  m.route_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.channel_type AS platform,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: DeleteMessagesByBot :exec
DELETE FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: ListObservedConversationsByChannelIdentity :many
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  COALESCE(r.conversation_type, '') AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(r.metadata->>'conversation_name', '')::text AS conversation_name,
  MAX(m.created_at)::timestamptz AS last_observed_at
FROM bot_history_messages m
JOIN bot_channel_routes r ON r.id = m.route_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.sender_channel_identity_id = sqlc.arg(channel_identity_id)
GROUP BY
  r.id,
  r.channel_type,
  r.conversation_type,
  r.external_conversation_id,
  r.external_thread_id,
  r.metadata
ORDER BY MAX(m.created_at) DESC;
