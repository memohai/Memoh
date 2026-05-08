-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  id, bot_id, session_id, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, model_id, event_id, display_text
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.narg(session_id),
  sqlc.narg(sender_channel_identity_id),
  sqlc.narg(sender_user_id),
  sqlc.narg(external_message_id),
  sqlc.narg(source_reply_to_message_id),
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.narg(model_id),
  sqlc.narg(event_id),
  sqlc.narg(display_text)
)
RETURNING
  id, bot_id, session_id, sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id, role, content, metadata, usage,
  event_id, display_text, created_at;

-- name: ListMessages :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at ASC
LIMIT 10000;

-- name: ListMessagesBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
ORDER BY m.created_at ASC
LIMIT 10000;

-- name: ListMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.created_at ASC;

-- name: ListMessagesSinceBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.created_at ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY m.created_at ASC;

-- name: ListActiveMessagesSinceBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY m.created_at ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatestBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: DeleteMessagesByBot :exec
DELETE FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesBySession :exec
DELETE FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id);

-- name: ListObservedConversationsByChannelIdentity :many
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at) AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND m.sender_channel_identity_id = sqlc.arg(channel_identity_id)
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(json_extract(r.metadata, '$.conversation_name'), '')), ''),
    NULLIF(TRIM(COALESCE(json_extract(r.metadata, '$.conversation_handle'), '')), ''),
    ''
  ) AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(json_extract(r.metadata, '$.conversation_avatar_url'), '')), ''), '') AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
ORDER BY rr.last_observed_at DESC;

-- name: ListObservedConversationsByChannelType :many
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at) AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  JOIN bot_channel_routes r ON r.id = s.route_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND lower(TRIM(r.channel_type)) = lower(TRIM(sqlc.arg(channel_type)))
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(json_extract(r.metadata, '$.conversation_name'), '')), ''),
    NULLIF(TRIM(COALESCE(json_extract(r.metadata, '$.conversation_handle'), '')), ''),
    ''
  ) AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(json_extract(r.metadata, '$.conversation_avatar_url'), '')), ''), '') AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
ORDER BY rr.last_observed_at DESC;

-- name: SearchMessages :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.role, m.content, m.created_at,
  ci.display_name AS sender_display_name,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(session_id) IS NULL OR m.session_id = sqlc.narg(session_id))
  AND (sqlc.narg(contact_id) IS NULL OR m.sender_channel_identity_id = sqlc.narg(contact_id))
  AND (sqlc.narg(start_time) IS NULL OR m.created_at >= sqlc.narg(start_time))
  AND (sqlc.narg(end_time) IS NULL OR m.created_at <= sqlc.narg(end_time))
  AND (sqlc.narg(role) IS NULL OR m.role = sqlc.narg(role))
  AND (sqlc.narg(keyword) IS NULL OR (
    CASE
      WHEN json_type(json_extract(m.content, '$.content')) = 'text'
        THEN json_extract(m.content, '$.content')
      WHEN json_type(json_extract(m.content, '$.content')) = 'array'
        THEN (SELECT COALESCE(group_concat(json_extract(j.value, '$.text'), ' '), '')
              FROM json_each(json_extract(m.content, '$.content')) AS j
              WHERE json_extract(j.value, '$.type') = 'text')
      ELSE ''
    END
  ) LIKE '%' || sqlc.narg(keyword) || '%')
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: MarkMessagesCompacted :exec
UPDATE bot_history_messages
SET compact_id = sqlc.arg(compact_id)
WHERE id IN (sqlc.slice(ids));

-- name: ListUncompactedMessagesBySession :many
SELECT id, bot_id, session_id, role, content, usage, sender_channel_identity_id, compact_id, created_at
FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id)
  AND compact_id IS NULL
ORDER BY created_at ASC;
