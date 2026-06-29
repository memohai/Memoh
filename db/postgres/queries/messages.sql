-- name: CreateHistoryTurn :one
WITH input AS (
  SELECT
    sqlc.arg(bot_id)::uuid AS bot_id,
    sqlc.narg(owner_session_id)::uuid AS owner_session_id,
    sqlc.narg(parent_turn_id)::uuid AS parent_turn_id
)
INSERT INTO bot_history_turns (
  bot_id,
  owner_session_id,
  parent_turn_id
)
SELECT bot_id, owner_session_id, parent_turn_id
FROM input
WHERE (
    owner_session_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = input.owner_session_id
        AND s.bot_id = input.bot_id
    )
  )
  AND (
    parent_turn_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns parent
      WHERE parent.id = input.parent_turn_id
        AND parent.bot_id = input.bot_id
    )
  )
RETURNING *;

-- name: UpdateHistoryTurnRequestMessage :one
UPDATE bot_history_turns
SET request_message_id = COALESCE(request_message_id, sqlc.narg(request_message_id)::uuid),
    updated_at = now()
WHERE bot_history_turns.id = sqlc.arg(id)
  AND (
    sqlc.narg(request_message_id)::uuid IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_messages m
      WHERE m.id = sqlc.narg(request_message_id)::uuid
        AND m.bot_id = bot_history_turns.bot_id
        AND m.role = 'user'
    )
  )
RETURNING *;

-- name: UpdateHistoryTurnFinalAssistantMessage :one
UPDATE bot_history_turns
SET final_assistant_message_id = sqlc.narg(final_assistant_message_id)::uuid,
    updated_at = now()
WHERE bot_history_turns.id = sqlc.arg(id)
  AND (
    sqlc.narg(final_assistant_message_id)::uuid IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_messages m
      WHERE m.id = sqlc.narg(final_assistant_message_id)::uuid
        AND m.bot_id = bot_history_turns.bot_id
        AND m.role = 'assistant'
    )
  )
RETURNING *;

-- name: GetNextTurnMessageSeq :one
SELECT (COALESCE(MAX(turn_message_seq), 0) + 1)::bigint AS next_seq
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
WITH RECURSIVE visible_turns AS (
  SELECT t.*, 0::bigint AS depth
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(head_turn_id)
  UNION ALL
  SELECT p.*, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  t.*
FROM visible_turns vt
JOIN bot_history_turns t ON t.id = vt.id
ORDER BY vt.depth ASC;

-- name: ListOtherActiveSessionVisibleTurnIDs :many
WITH RECURSIVE visible_turns AS (
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
WITH RECURSIVE graph_turns AS (
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
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
WITH input AS (
  SELECT
    sqlc.arg(bot_id)::uuid AS bot_id,
    sqlc.narg(session_id)::uuid AS session_id,
    sqlc.narg(turn_id)::uuid AS turn_id,
    sqlc.narg(turn_message_seq)::bigint AS turn_message_seq,
    sqlc.narg(sender_channel_identity_id)::uuid AS sender_channel_identity_id,
    sqlc.narg(sender_user_id)::uuid AS sender_user_id,
    sqlc.narg(external_message_id)::text AS external_message_id,
    sqlc.narg(source_reply_to_message_id)::text AS source_reply_to_message_id,
    sqlc.arg(role)::text AS role,
    sqlc.arg(content)::jsonb AS content,
    sqlc.arg(metadata)::jsonb AS metadata,
    sqlc.arg(usage)::jsonb AS usage,
    sqlc.narg(model_id)::uuid AS model_id,
    sqlc.narg(event_id)::uuid AS event_id,
    sqlc.narg(display_text)::text AS display_text
)
INSERT INTO bot_history_messages (
  bot_id,
  session_id,
  turn_id,
  turn_message_seq,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  model_id,
  event_id,
  display_text
)
SELECT
  bot_id,
  session_id,
  turn_id,
  turn_message_seq,
  sender_channel_identity_id,
  sender_user_id,
  external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  model_id,
  event_id,
  display_text
FROM input
WHERE (
    session_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = input.session_id
        AND s.bot_id = input.bot_id
    )
  )
  AND (
    turn_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns t
      WHERE t.id = input.turn_id
        AND t.bot_id = input.bot_id
    )
  )
RETURNING
  id,
  bot_id,
  session_id,
  turn_id,
  turn_message_seq,
  sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  event_id,
  display_text,
  created_at;

-- name: ListMessages :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
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
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC
LIMIT 10000;

-- name: ListSessionTurnGraphNodeMetadata :many
WITH RECURSIVE graph_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions bs
  JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN graph_turns gt ON gt.parent_turn_id = p.id
),
request_assets AS (
  SELECT
    a.message_id,
    string_agg(
      concat_ws(
        ':',
        COALESCE(a.content_hash, ''),
        COALESCE(a.name, ''),
        COALESCE(a.role, ''),
        COALESCE(a.ordinal::text, '')
      ),
      '|'
      ORDER BY a.content_hash, a.name, a.role, a.ordinal, a.id
    ) AS request_asset_key
  FROM bot_history_message_assets a
  GROUP BY a.message_id
)
SELECT
  gt.id AS turn_id,
  COALESCE(MIN(m.created_at), t.created_at) AS node_created_at,
  COALESCE(rm.content, 'null'::jsonb) AS request_content,
  COALESCE(rm.display_text, '')::text AS request_display_text,
  COALESCE(ra.request_asset_key, '')::text AS request_asset_key,
  (t.request_message_id IS NOT NULL)::boolean AS has_user,
  EXISTS (
    SELECT 1
    FROM bot_history_messages assistant_m
    WHERE assistant_m.turn_id = gt.id
      AND assistant_m.role = 'assistant'
  ) AS has_assistant
FROM graph_turns gt
JOIN bot_history_turns t ON t.id = gt.id
LEFT JOIN bot_history_messages m ON m.turn_id = gt.id
LEFT JOIN bot_history_messages rm ON rm.id = t.request_message_id
LEFT JOIN request_assets ra ON ra.message_id = t.request_message_id
GROUP BY gt.id, t.created_at, rm.content, rm.display_text, ra.request_asset_key
ORDER BY COALESCE(MIN(m.created_at), t.created_at) ASC, gt.id ASC;

-- name: ListMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
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
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.created_at >= sqlc.arg(created_at)
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY m.created_at ASC;

-- name: ListActiveMessagesSinceBySession :many
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSinceByTurn :many
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(head_turn_id)
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
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
WITH RECURSIVE selected_head AS (
  SELECT
    bs.id AS session_id,
    CASE
      WHEN sqlc.narg(head_turn_id)::uuid IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END AS head_turn_id
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)::uuid
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(before_id)::uuid
LEFT JOIN visible_turns cursor_turn ON cursor_turn.id = cursor_message.turn_id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE (
  (
    sqlc.narg(before_id)::uuid IS NOT NULL
    AND cursor_turn.id IS NOT NULL
    AND (
      (-vt.depth, COALESCE(m.turn_message_seq, 0)::bigint, m.created_at, m.id)
      < (-cursor_turn.depth, COALESCE(cursor_message.turn_message_seq, 0)::bigint, cursor_message.created_at, cursor_message.id)
    )
  )
  OR (
    sqlc.narg(before_id)::uuid IS NULL
    AND m.created_at < sqlc.arg(created_at)
  )
)
ORDER BY vt.depth ASC, COALESCE(m.turn_message_seq, 9223372036854775807) DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
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
WITH RECURSIVE selected_head AS (
  SELECT
    bs.id AS session_id,
    CASE
      WHEN sqlc.narg(head_turn_id)::uuid IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END AS head_turn_id
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)::uuid
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
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
WITH RECURSIVE selected_head AS (
  SELECT
    bs.id AS session_id,
    CASE
      WHEN sqlc.narg(head_turn_id)::uuid IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END AS head_turn_id
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)::uuid
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
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
WITH RECURSIVE selected_head AS (
  SELECT
    bs.id AS session_id,
    CASE
      WHEN sqlc.narg(head_turn_id)::uuid IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END AS head_turn_id
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = sqlc.narg(head_turn_id)::uuid
  WHERE bs.id = sqlc.arg(session_id)
    AND bs.deleted_at IS NULL
), visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN bot_history_messages cursor_message ON cursor_message.id = sqlc.narg(after_id)::uuid
LEFT JOIN visible_turns cursor_turn ON cursor_turn.id = cursor_message.turn_id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE (
  (
    sqlc.narg(after_id)::uuid IS NOT NULL
    AND cursor_turn.id IS NOT NULL
    AND (
      (-vt.depth, COALESCE(m.turn_message_seq, 0)::bigint, m.created_at, m.id)
      > (-cursor_turn.depth, COALESCE(cursor_message.turn_message_seq, 0)::bigint, cursor_message.created_at, cursor_message.id)
    )
  )
  OR (
    sqlc.narg(after_id)::uuid IS NULL
    AND m.created_at > sqlc.arg(created_at)
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
    updated_at = now()
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
    MAX(m.created_at)::timestamptz AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND m.sender_channel_identity_id = sqlc.arg(channel_identity_id)::uuid
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_name', '')), ''),
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_handle', '')), ''),
    ''
  )::text AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(r.metadata->>'conversation_avatar_url', '')), ''), '')::text AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
GROUP BY
  r.id,
  r.channel_type,
  r.conversation_type,
  r.external_conversation_id,
  r.external_thread_id,
  r.metadata,
  rr.last_observed_at
ORDER BY rr.last_observed_at DESC;

-- name: ListObservedConversationsByChannelType :many
-- Routes on this platform type where the bot has seen at least one message (any sender).
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at)::timestamptz AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  JOIN bot_channel_routes r ON r.id = s.route_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND LOWER(TRIM(r.channel_type)) = LOWER(TRIM(sqlc.arg(channel_type)))
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_name', '')), ''),
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_handle', '')), ''),
    ''
  )::text AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(r.metadata->>'conversation_avatar_url', '')), ''), '')::text AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
GROUP BY
  r.id,
  r.channel_type,
  r.conversation_type,
  r.external_conversation_id,
  r.external_thread_id,
  r.metadata,
  rr.last_observed_at
ORDER BY rr.last_observed_at DESC;

-- name: SearchMessages :many
-- Session-scoped search follows bot_sessions.default_head_turn_id. The client
-- selected variant is intentionally not part of this tool/query contract.
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
  FROM bot_sessions bs
  JOIN bot_history_turns t ON t.id = bs.default_head_turn_id
    AND t.bot_id = bs.bot_id
  WHERE sqlc.narg(session_id)::uuid IS NOT NULL
    AND bs.id = sqlc.narg(session_id)::uuid
    AND bs.deleted_at IS NULL
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.role,
  m.content,
  m.created_at,
  ci.display_name AS sender_display_name,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(session_id)::uuid IS NULL OR m.turn_id IN (SELECT id FROM visible_turns))
  AND (sqlc.narg(contact_id)::uuid IS NULL OR m.sender_channel_identity_id = sqlc.narg(contact_id)::uuid)
  AND (sqlc.narg(start_time)::timestamptz IS NULL OR m.created_at >= sqlc.narg(start_time)::timestamptz)
  AND (sqlc.narg(end_time)::timestamptz IS NULL OR m.created_at <= sqlc.narg(end_time)::timestamptz)
  AND (sqlc.narg(role)::text IS NULL OR m.role = sqlc.narg(role)::text)
  AND (sqlc.narg(keyword)::text IS NULL OR (
    CASE
      WHEN jsonb_typeof(m.content->'content') = 'string'
        THEN m.content->>'content'
      WHEN jsonb_typeof(m.content->'content') = 'array'
        THEN (SELECT COALESCE(string_agg(elem->>'text', ' '), '')
              FROM jsonb_array_elements(m.content->'content') AS elem
              WHERE elem->>'type' = 'text')
      ELSE ''
    END
  ) ILIKE '%' || sqlc.narg(keyword)::text || '%')
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: MarkMessagesCompacted :exec
UPDATE bot_history_messages
SET compact_id = $1
WHERE id = ANY($2::uuid[]);

-- name: ListUncompactedMessagesBySession :many
-- Compaction uses the session's server-canonical default head, not a client's
-- transient selected variant.
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
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
