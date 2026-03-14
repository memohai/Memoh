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
WITH observed_routes AS (
  SELECT
    (i.header->>'route_id')::uuid AS route_id,
    MAX(i.created_at)::timestamptz AS last_observed_at
  FROM bot_inbox i
  WHERE i.bot_id = sqlc.arg(bot_id)
    AND i.header->>'channel-identity-id' = sqlc.arg(channel_identity_id)::text
    AND COALESCE(i.header->>'route_id', '') != ''
  GROUP BY (i.header->>'route_id')::uuid

  UNION ALL

  SELECT
    m.route_id,
    MAX(m.created_at)::timestamptz AS last_observed_at
  FROM bot_history_messages m
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND m.sender_channel_identity_id = sqlc.arg(channel_identity_id)::uuid
    AND m.route_id IS NOT NULL
  GROUP BY m.route_id
),
ranked_routes AS (
  SELECT
    route_id,
    MAX(last_observed_at)::timestamptz AS last_observed_at
  FROM observed_routes
  GROUP BY route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(r.metadata->>'conversation_name', '')::text AS conversation_name,
  rr.last_observed_at
FROM ranked_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
WHERE LOWER(COALESCE(r.conversation_type, '')) NOT IN ('', 'p2p', 'private', 'direct', 'dm')
GROUP BY
  r.id,
  r.channel_type,
  r.conversation_type,
  r.external_conversation_id,
  r.external_thread_id,
  r.metadata,
  rr.last_observed_at
ORDER BY rr.last_observed_at DESC;
