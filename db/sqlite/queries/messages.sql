-- name: CreateHistoryTurn :one
INSERT INTO bot_history_turns (
  id,
  bot_id,
  owner_session_id,
  parent_turn_id,
  origin_kind,
  origin_turn_id,
  request_group_id
)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.narg(owner_session_id),
  sqlc.narg(parent_turn_id),
  sqlc.narg(origin_kind),
  sqlc.narg(origin_turn_id),
  sqlc.narg(request_group_id)
WHERE (
    sqlc.narg(owner_session_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = sqlc.narg(owner_session_id)
        AND s.bot_id = sqlc.arg(bot_id)
    )
  )
  AND (
    sqlc.narg(parent_turn_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns parent
      WHERE parent.id = sqlc.narg(parent_turn_id)
        AND parent.bot_id = sqlc.arg(bot_id)
    )
  )
RETURNING *;

-- name: UpdateHistoryTurnRequestMessage :one
UPDATE bot_history_turns
SET request_message_id = COALESCE(request_message_id, sqlc.narg(request_message_id)),
    updated_at = CURRENT_TIMESTAMP
WHERE bot_history_turns.id = sqlc.arg(id)
  AND (
    sqlc.narg(request_message_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_messages m
      WHERE m.id = sqlc.narg(request_message_id)
        AND m.bot_id = bot_history_turns.bot_id
        AND m.role = 'user'
    )
  )
RETURNING *;

-- name: UpdateHistoryTurnFinalAssistantMessage :one
UPDATE bot_history_turns
SET final_assistant_message_id = sqlc.narg(final_assistant_message_id),
    updated_at = CURRENT_TIMESTAMP
WHERE bot_history_turns.id = sqlc.arg(id)
  AND (
    sqlc.narg(final_assistant_message_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_messages m
      WHERE m.id = sqlc.narg(final_assistant_message_id)
        AND m.bot_id = bot_history_turns.bot_id
        AND m.role = 'assistant'
    )
  )
RETURNING *;

-- name: GetNextTurnMessageSeq :one
SELECT COALESCE(MAX(turn_message_seq), 0) + 1 AS next_seq
FROM bot_history_messages
WHERE turn_id = sqlc.arg(turn_id);

-- name: GetHistoryTurnByID :one
SELECT *
FROM bot_history_turns
WHERE id = sqlc.arg(id);

-- name: ListSessionOwnedTurnsForCleanup :many
SELECT t.*
FROM bot_history_turns t
WHERE t.owner_session_id = sqlc.arg(session_id)
ORDER BY t.created_at DESC, t.id DESC;

-- name: ListHistoryTurnPathFromHead :many
WITH RECURSIVE visible_turns(
  id,
  bot_id,
  owner_session_id,
  parent_turn_id,
  request_message_id,
  final_assistant_message_id,
  created_at,
  updated_at,
  depth
) AS (
  SELECT
    t.id,
    t.bot_id,
    t.owner_session_id,
    t.parent_turn_id,
    t.request_message_id,
    t.final_assistant_message_id,
    t.created_at,
    t.updated_at,
    0
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(head_turn_id)
  UNION ALL
  SELECT
    p.id,
    p.bot_id,
    p.owner_session_id,
    p.parent_turn_id,
    p.request_message_id,
    p.final_assistant_message_id,
    p.created_at,
    p.updated_at,
    vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  t.*
FROM visible_turns vt
JOIN bot_history_turns t ON t.id = vt.id
ORDER BY vt.depth ASC;

-- name: ListOtherActiveSessionVisibleTurnIDs :many
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  JOIN bot_sessions source ON source.id = sqlc.arg(session_id)
  WHERE s.id <> source.id
    AND s.bot_id = source.bot_id
    AND s.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT DISTINCT visible_turns.id
FROM visible_turns;

-- name: ListSessionTurnGraphTurns :many
WITH RECURSIVE graph_turns(id, parent_turn_id) AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = sqlc.arg(session_id)
    AND s.deleted_at IS NULL
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN graph_turns gt ON gt.parent_turn_id = p.id
)
SELECT t.*
FROM graph_turns gt
JOIN bot_history_turns t ON t.id = gt.id
ORDER BY t.created_at ASC, t.id ASC;

-- name: DeleteMessagesByTurnID :exec
DELETE FROM bot_history_messages
WHERE turn_id = sqlc.arg(turn_id);

-- name: DeleteHistoryTurnByID :exec
DELETE FROM bot_history_turns
WHERE id = sqlc.arg(id);

-- name: GetVisibleAssistantMessageTurnForFork :one
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
    AND h.head_turn_id = sqlc.arg(base_head_turn_id)
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = sqlc.arg(session_id)
    AND s.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id AS message_id,
  m.role,
  m.turn_id,
  t.parent_turn_id
FROM bot_history_messages m
JOIN visible_turns vt ON vt.id = m.turn_id
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.id = sqlc.arg(message_id)
  AND m.role = 'assistant'
LIMIT 1;

-- name: GetVisibleAssistantTurnForRetry :one
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
    AND h.head_turn_id = sqlc.arg(base_head_turn_id)
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = sqlc.arg(session_id)
    AND s.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  assistant.id AS assistant_message_id,
  assistant.turn_id,
  t.parent_turn_id,
  COALESCE(t.request_group_id, t.id) AS request_group_id,
  request.id AS request_message_id,
  request.content AS request_content,
  request.display_text AS request_display_text
FROM bot_history_messages assistant
JOIN visible_turns vt ON vt.id = assistant.turn_id
JOIN bot_history_turns t ON t.id = assistant.turn_id
JOIN bot_history_messages request ON request.id = t.request_message_id
WHERE assistant.id = sqlc.arg(message_id)
  AND assistant.role = 'assistant'
  AND request.role = 'user'
LIMIT 1;

-- name: GetVisibleUserMessageTurnForRewrite :one
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
    AND h.head_turn_id = sqlc.arg(base_head_turn_id)
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = sqlc.arg(session_id)
    AND s.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id AS message_id,
  m.turn_id,
  t.parent_turn_id
FROM bot_history_messages m
JOIN visible_turns vt ON vt.id = m.turn_id
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.id = sqlc.arg(message_id)
  AND m.role = 'user'
LIMIT 1;

-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_message_seq, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, session_mode, runtime_type, model_id, event_id, display_text
)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.narg(session_id),
  sqlc.narg(turn_id),
  sqlc.narg(turn_message_seq),
  sqlc.narg(sender_channel_identity_id),
  sqlc.narg(sender_user_id),
  sqlc.narg(external_message_id),
  sqlc.narg(source_reply_to_message_id),
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.arg(session_mode),
  sqlc.arg(runtime_type),
  sqlc.narg(model_id),
  sqlc.narg(event_id),
  sqlc.narg(display_text)
WHERE (
    sqlc.narg(session_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = sqlc.narg(session_id)
        AND s.bot_id = sqlc.arg(bot_id)
    )
  )
  AND (
    sqlc.narg(turn_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns t
      WHERE t.id = sqlc.narg(turn_id)
        AND t.bot_id = sqlc.arg(bot_id)
    )
  )
RETURNING
  id, bot_id, session_id, turn_id, turn_message_seq, sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id, role, content, metadata, usage, session_mode, runtime_type,
  event_id, display_text, created_at;

-- name: ListMessages :many
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
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
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC
LIMIT 10000;

-- name: ResolveSessionTurnHead :one
-- Resolve any turn id to a session head whose ancestor path contains it
-- (a head contains the turn exactly when the head is one of the turn's
-- descendants, self included), preferring the most recently updated head.
-- Used when a client pages with a non-head turn id (variant switching);
-- true heads short-circuit through the cheap heads-table lookup before this
-- recursion runs.
WITH RECURSIVE descendant_turns(id) AS (
  SELECT t.id
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(target_turn_id)
  UNION ALL
  SELECT c.id
  FROM bot_history_turns c
  JOIN descendant_turns dt ON c.parent_turn_id = dt.id
)
SELECT h.head_turn_id
FROM bot_session_turn_heads h
WHERE h.session_id = sqlc.arg(session_id)
  AND h.head_turn_id IN (SELECT dt.id FROM descendant_turns dt)
ORDER BY h.updated_at DESC, h.head_turn_id DESC
LIMIT 1;

-- name: ListSessionTurnSiblings :many
-- Variant metadata for one transcript page: every turn reachable from the
-- session's active heads that shares a parent with one of the page turns
-- (root page turns pair with the other reachable roots). Restricting
-- candidates to the reachable set keeps forks made in other sessions out of
-- the variant switcher.
WITH RECURSIVE session_turns(id, parent_turn_id) AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_session_turn_heads h
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE h.session_id = sqlc.arg(session_id)
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN session_turns st ON st.parent_turn_id = p.id
),
page_parents(parent_turn_id) AS (
  SELECT DISTINCT t.parent_turn_id
  FROM bot_history_turns t
  WHERE t.id IN (sqlc.slice(turn_ids))
)
SELECT
  t.id AS turn_id,
  t.parent_turn_id,
  COALESCE(t.request_group_id, t.id) AS request_group_id,
  t.request_message_id IS NOT NULL AS has_user,
  t.final_assistant_message_id IS NOT NULL AS has_assistant
FROM bot_history_turns t
JOIN session_turns st ON st.id = t.id
WHERE (
    t.parent_turn_id IS NOT NULL
    AND t.parent_turn_id IN (
      SELECT pp.parent_turn_id FROM page_parents pp WHERE pp.parent_turn_id IS NOT NULL
    )
  )
  OR (
    t.parent_turn_id IS NULL
    AND EXISTS (
      SELECT 1 FROM page_parents pp WHERE pp.parent_turn_id IS NULL
    )
  )
ORDER BY t.created_at ASC, t.id ASC;

-- name: ListSessionTurnPathIDs :many
-- Ancestor path ids (self included) of one head turn. Used by the SSE stream
-- to decide whether a live message belongs to the subscribed head path.
WITH RECURSIVE path_turns(id, parent_turn_id) AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(head_turn_id)
  UNION ALL
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN path_turns pt ON pt.parent_turn_id = p.id
)
SELECT pt.id
FROM path_turns pt;

-- name: ListMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
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
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
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
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSinceByTurn :many
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(head_turn_id)
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
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
WITH RECURSIVE selected_head(session_id, head_turn_id) AS (
  SELECT
    bs.id,
    CASE
      WHEN sqlc.narg(head_turn_id) IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), head_path(id, parent_turn_id, depth, found_cursor) AS (
  SELECT
    t.id,
    t.parent_turn_id,
    0 AS depth,
    (
      sqlc.narg(before_id) IS NOT NULL
      AND t.id = cursor_message.turn_id
    ) AS found_cursor
  FROM selected_head sh
  JOIN bot_history_turns t ON t.id = sh.head_turn_id
  JOIN bot_sessions bs ON bs.id = sh.session_id
    AND bs.bot_id = t.bot_id
  LEFT JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(before_id)
  UNION ALL
  SELECT
    p.id,
    p.parent_turn_id,
    hp.depth + 1 AS depth,
    (
      sqlc.narg(before_id) IS NOT NULL
      AND p.id = cursor_message.turn_id
    ) AS found_cursor
  FROM bot_history_turns p
  JOIN head_path hp ON hp.parent_turn_id = p.id
  LEFT JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(before_id)
  WHERE sqlc.narg(before_id) IS NOT NULL
    AND NOT hp.found_cursor
), cursor_turn(id, parent_turn_id, depth) AS (
  SELECT hp.id, hp.parent_turn_id, hp.depth
  FROM head_path hp
  JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(before_id)
    AND cursor_message.turn_id = hp.id
  WHERE sqlc.narg(before_id) IS NOT NULL
), visible_turns(id, parent_turn_id, depth, covered_messages) AS (
  SELECT
    hp.id,
    hp.parent_turn_id,
    hp.depth,
    (
      SELECT COUNT(*)
      FROM bot_history_messages count_m
      WHERE count_m.turn_id = hp.id
        AND count_m.created_at < strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
    )
  FROM head_path hp
  WHERE sqlc.narg(before_id) IS NULL
    AND hp.depth = 0
  UNION ALL
  SELECT
    ct.id,
    ct.parent_turn_id,
    ct.depth,
    (
      SELECT COUNT(*)
      FROM bot_history_messages count_m
      JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(before_id)
      WHERE count_m.turn_id = ct.id
        AND (
          COALESCE(count_m.turn_message_seq, 0) < COALESCE(cursor_message.turn_message_seq, 0)
          OR (COALESCE(count_m.turn_message_seq, 0) = COALESCE(cursor_message.turn_message_seq, 0) AND count_m.created_at < cursor_message.created_at)
          OR (COALESCE(count_m.turn_message_seq, 0) = COALESCE(cursor_message.turn_message_seq, 0) AND count_m.created_at = cursor_message.created_at AND count_m.id < cursor_message.id)
        )
    )
  FROM cursor_turn ct
  WHERE sqlc.narg(before_id) IS NOT NULL
  UNION ALL
  SELECT
    p.id,
    p.parent_turn_id,
    vt.depth + 1,
    vt.covered_messages + (
      SELECT COUNT(*)
      FROM bot_history_messages count_m
      WHERE count_m.turn_id = p.id
        AND (
          sqlc.narg(before_id) IS NOT NULL
          OR count_m.created_at < strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
        )
    )
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
  WHERE vt.covered_messages < sqlc.arg(max_count)
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(before_id)
LEFT JOIN cursor_turn ON cursor_turn.id = cursor_message.turn_id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE (
  (
    sqlc.narg(before_id) IS NOT NULL
    AND cursor_turn.id IS NOT NULL
    AND (
      -vt.depth < -cursor_turn.depth
      OR (-vt.depth = -cursor_turn.depth AND COALESCE(m.turn_message_seq, 0) < COALESCE(cursor_message.turn_message_seq, 0))
      OR (-vt.depth = -cursor_turn.depth AND COALESCE(m.turn_message_seq, 0) = COALESCE(cursor_message.turn_message_seq, 0) AND m.created_at < cursor_message.created_at)
      OR (-vt.depth = -cursor_turn.depth AND COALESCE(m.turn_message_seq, 0) = COALESCE(cursor_message.turn_message_seq, 0) AND m.created_at = cursor_message.created_at AND m.id < cursor_message.id)
    )
  )
  OR (
    sqlc.narg(before_id) IS NULL
    AND m.created_at < strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  )
)
ORDER BY vt.depth ASC, COALESCE(m.turn_message_seq, 9223372036854775807) DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
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
WITH RECURSIVE selected_head(session_id, head_turn_id) AS (
  SELECT
    bs.id,
    CASE
      WHEN sqlc.narg(head_turn_id) IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), visible_turns(id, parent_turn_id, depth, covered_messages) AS (
  SELECT
    t.id,
    t.parent_turn_id,
    0,
    (
      SELECT COUNT(*)
      FROM bot_history_messages count_m
      WHERE count_m.turn_id = t.id
    )
  FROM selected_head sh
  JOIN bot_history_turns t ON t.id = sh.head_turn_id
  JOIN bot_sessions bs ON bs.id = sh.session_id
    AND bs.bot_id = t.bot_id
  UNION ALL
  SELECT
    p.id,
    p.parent_turn_id,
    vt.depth + 1,
    vt.covered_messages + (
      SELECT COUNT(*)
      FROM bot_history_messages count_m
      WHERE count_m.turn_id = p.id
    )
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
  WHERE vt.covered_messages < sqlc.arg(max_count)
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
ORDER BY vt.depth ASC, COALESCE(m.turn_message_seq, 9223372036854775807) DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: GetMessageByExternalIDBySession :one
WITH RECURSIVE selected_head(session_id, head_turn_id) AS (
  SELECT
    bs.id,
    CASE
      WHEN sqlc.narg(head_turn_id) IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM selected_head sh
  JOIN bot_history_turns t ON t.id = sh.head_turn_id
  JOIN bot_sessions bs ON bs.id = sh.session_id
    AND bs.bot_id = t.bot_id
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.source_message_id = sqlc.arg(external_message_id)
ORDER BY vt.depth ASC, COALESCE(m.turn_message_seq, 9223372036854775807) DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: ListMessagesAfterBySession :many
WITH RECURSIVE selected_head(session_id, head_turn_id) AS (
  SELECT
    bs.id,
    CASE
      WHEN sqlc.narg(head_turn_id) IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM selected_head sh
  JOIN bot_history_turns t ON t.id = sh.head_turn_id
  JOIN bot_sessions bs ON bs.id = sh.session_id
    AND bs.bot_id = t.bot_id
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.turn_id, m.turn_message_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(after_id)
LEFT JOIN visible_turns cursor_turn ON cursor_turn.id = cursor_message.turn_id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE (
  (
    sqlc.narg(after_id) IS NOT NULL
    AND cursor_turn.id IS NOT NULL
    AND (
      -vt.depth > -cursor_turn.depth
      OR (-vt.depth = -cursor_turn.depth AND COALESCE(m.turn_message_seq, 0) > COALESCE(cursor_message.turn_message_seq, 0))
      OR (-vt.depth = -cursor_turn.depth AND COALESCE(m.turn_message_seq, 0) = COALESCE(cursor_message.turn_message_seq, 0) AND m.created_at > cursor_message.created_at)
      OR (-vt.depth = -cursor_turn.depth AND COALESCE(m.turn_message_seq, 0) = COALESCE(cursor_message.turn_message_seq, 0) AND m.created_at = cursor_message.created_at AND m.id > cursor_message.id)
    )
  )
  OR (
    sqlc.narg(after_id) IS NULL
    AND m.created_at > strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  )
)
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: CountMessagesByBot :one
SELECT COUNT(*) FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesByBot :exec
DELETE FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: ClearHistoryTurnMessagePointersByBot :exec
UPDATE bot_history_turns
SET request_message_id = NULL,
    final_assistant_message_id = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteHistoryTurnsByBot :exec
DELETE FROM bot_history_turns
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
-- Session-scoped search follows bot_sessions.default_head_turn_id. The client
-- selected variant is intentionally not part of this tool/query contract.
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE sqlc.narg(session_id) IS NOT NULL
    AND bs.id = sqlc.narg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.role, m.content, m.created_at,
  ci.display_name AS sender_display_name,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(session_id) IS NULL OR m.turn_id IN (SELECT id FROM visible_turns))
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
-- Compaction uses the session's server-canonical default head, not a client's
-- transient selected variant.
WITH RECURSIVE visible_turns(id, parent_turn_id, depth) AS (
  SELECT t.id, t.parent_turn_id, 0
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT m.id, m.bot_id, m.session_id, m.role, m.content, m.usage, m.sender_channel_identity_id, m.compact_id, m.created_at
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
WHERE m.compact_id IS NULL
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC;
