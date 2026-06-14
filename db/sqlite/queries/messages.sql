-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  id, bot_id, session_id, branch_id, branch_seq, sender_channel_identity_id, sender_account_user_id,
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
  sqlc.narg(branch_id),
  CASE
    WHEN sqlc.narg(branch_id) IS NULL THEN NULL
    ELSE (
      SELECT COALESCE(MAX(existing.branch_seq), 0) + 1
      FROM bot_history_messages existing
      WHERE existing.branch_id = sqlc.narg(branch_id)
    )
  END,
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
  id, bot_id, session_id, branch_id, branch_seq, sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id, role, content, metadata, usage,
  event_id, display_text, created_at;

-- name: ListMessages :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT
    b.id,
    b.parent_branch_id,
    NULL,
    0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(
    s.active_branch_id,
    (
      SELECT rb.id
      FROM bot_session_branches rb
      WHERE rb.session_id = s.id
        AND rb.parent_branch_id IS NULL
        AND rb.fork_from_message_id IS NULL
      ORDER BY rb.created_at ASC
      LIMIT 1
    )
  )
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC
LIMIT 10000;

-- name: ListMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
ORDER BY m.created_at ASC;

-- name: ListMessagesSinceBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY m.created_at ASC;

-- name: ListActiveMessagesSinceBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
  AND m.created_at < strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at < strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
ORDER BY COALESCE(bp.depth, -1) ASC, COALESCE(m.branch_seq, 0) DESC, m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
ORDER BY COALESCE(bp.depth, -1) ASC, COALESCE(m.branch_seq, 0) DESC, m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: GetMessageByExternalIDBySession :one
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.source_message_id = sqlc.arg(external_message_id)
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
ORDER BY COALESCE(bp.depth, -1) ASC, COALESCE(m.branch_seq, 0) DESC, m.created_at DESC
LIMIT 1;

-- name: ListMessagesAfterBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
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
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at > strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC
LIMIT sqlc.arg(max_count);

-- name: CountMessagesByBot :one
SELECT COUNT(*) FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

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
  AND (sqlc.narg(start_time) IS NULL OR m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.narg(start_time)))
  AND (sqlc.narg(end_time) IS NULL OR m.created_at <= strftime('%Y-%m-%d %H:%M:%S', sqlc.narg(end_time)))
  AND (sqlc.narg(role) IS NULL OR m.role = sqlc.narg(role))
  AND (sqlc.narg(keyword) IS NULL OR (
    CASE
      WHEN NOT json_valid(m.content)
        THEN CASE WHEN m.content LIKE '%' || sqlc.narg(keyword) || '%' THEN m.content ELSE '' END
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
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
)
SELECT m.id, m.bot_id, m.session_id, m.role, m.content, m.usage, m.sender_channel_identity_id, m.compact_id, m.created_at
FROM bot_history_messages m
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.compact_id IS NULL
  AND (
    m.branch_id IS NULL
    OR (bp.branch_id IS NOT NULL AND (bp.max_seq IS NULL OR m.branch_seq <= bp.max_seq))
  )
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;


-- name: GetActiveSessionBranch :one
SELECT active_branch_id
FROM bot_sessions
WHERE id = sqlc.arg(session_id);

-- name: SetActiveSessionBranch :exec
UPDATE bot_sessions
SET active_branch_id = sqlc.arg(branch_id),
    updated_at = CURRENT_TIMESTAMP
WHERE bot_sessions.id = sqlc.arg(session_id)
  AND EXISTS (
    SELECT 1
    FROM bot_session_branches b
    WHERE b.id = sqlc.arg(branch_id)
      AND b.session_id = sqlc.arg(session_id)
  );

-- name: GetRootSessionBranch :one
SELECT id
FROM bot_session_branches
WHERE session_id = sqlc.arg(session_id)
  AND parent_branch_id IS NULL
  AND fork_from_message_id IS NULL
ORDER BY created_at ASC
LIMIT 1;

-- name: CreateRootSessionBranch :one
INSERT INTO bot_session_branches (id, session_id)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(session_id)
)
RETURNING id;

-- name: GetMessageForSessionBranchFork :one
SELECT
  m.id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
  m.role,
  m.created_at
FROM bot_history_messages m
WHERE m.id = sqlc.arg(message_id)
  AND m.session_id = sqlc.arg(session_id)
LIMIT 1;

-- name: CreateSessionBranchFromMessage :one
INSERT INTO bot_session_branches (
  id,
  session_id,
  parent_branch_id,
  fork_from_message_id,
  fork_from_seq,
  title
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(session_id),
  sqlc.arg(parent_branch_id),
  sqlc.arg(fork_from_message_id),
  sqlc.arg(fork_from_seq),
  sqlc.narg(title)
)
RETURNING id;

-- name: ListSessionBranches :many
SELECT
  b.id,
  b.session_id,
  b.parent_branch_id,
  b.fork_from_message_id,
  b.fork_from_seq,
  b.title,
  b.created_at,
  b.updated_at,
  s.active_branch_id
FROM bot_session_branches b
JOIN bot_sessions s ON s.id = b.session_id
WHERE b.session_id = sqlc.arg(session_id)
ORDER BY b.created_at ASC, b.id ASC;

-- name: ListSessionBranchPreviewMessages :many
WITH targets AS (
  SELECT
    b.id AS branch_id,
    CASE
      WHEN b.fork_from_seq IS NOT NULL THEN b.parent_branch_id
      ELSE b.id
    END AS preview_branch_id,
    COALESCE(b.fork_from_seq, (
      SELECT MIN(m.branch_seq)
      FROM bot_history_messages m
      WHERE m.branch_id = b.id
        AND m.role = 'assistant'
    )) AS preview_seq
  FROM bot_session_branches b
  WHERE b.session_id = sqlc.arg(session_id)
)
SELECT
  m.id, m.bot_id, m.session_id, t.branch_id AS branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM targets t
JOIN bot_history_messages m ON m.branch_id = t.preview_branch_id
  AND m.branch_seq IN (t.preview_seq - 1, t.preview_seq)
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE t.preview_branch_id IS NOT NULL AND t.preview_seq IS NOT NULL
ORDER BY m.branch_id, m.branch_seq ASC, m.created_at ASC;

-- name: ListSessionBranchTurnMessages :many
SELECT
  a.id AS assistant_id,
  a.bot_id,
  a.session_id,
  a.branch_id,
  a.branch_seq,
  a.content AS assistant_content,
  a.display_text AS assistant_display_text,
  a.created_at AS assistant_created_at,
  u.id AS user_id,
  u.content AS user_content,
  u.display_text AS user_display_text,
  u.created_at AS user_created_at
FROM bot_session_branches b
JOIN bot_history_messages a ON a.branch_id = b.id
  AND a.role = 'assistant'
LEFT JOIN bot_history_messages u ON u.branch_id = a.branch_id
  AND u.branch_seq = a.branch_seq - 1
  AND u.role = 'user'
WHERE b.session_id = sqlc.arg(session_id)
ORDER BY b.created_at ASC, a.branch_seq ASC, a.created_at ASC;
