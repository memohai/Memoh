-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  model_id,
  event_id,
  display_text
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(session_id)::uuid,
  sqlc.narg(sender_channel_identity_id)::uuid,
  sqlc.narg(sender_user_id)::uuid,
  sqlc.narg(external_message_id)::text,
  sqlc.narg(source_reply_to_message_id)::text,
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.arg(session_mode),
  sqlc.arg(runtime_type),
  sqlc.narg(model_id)::uuid,
  sqlc.narg(event_id)::uuid,
  sqlc.narg(display_text)::text
)
RETURNING
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  event_id,
  display_text,
  created_at;

-- name: CreateHistoryTurn :one
INSERT INTO bot_history_turns (
  bot_id,
  session_id,
  position,
  request_message_id,
  assistant_message_id
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  COALESCE((
    SELECT MAX(position) + 1
    FROM bot_history_turns
    WHERE session_id = sqlc.arg(session_id)
  ), 1),
  sqlc.narg(request_message_id)::uuid,
  sqlc.narg(assistant_message_id)::uuid
)
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: BindHistoryTurnAssistantByRequest :one
UPDATE bot_history_turns
SET assistant_message_id = sqlc.arg(assistant_message_id),
    updated_at = now()
WHERE session_id = sqlc.arg(session_id)
  AND request_message_id = sqlc.arg(request_message_id)
  AND assistant_message_id IS NULL
  AND superseded_at IS NULL
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: BindLatestHistoryTurnAssistant :one
UPDATE bot_history_turns
SET assistant_message_id = sqlc.arg(assistant_message_id),
    updated_at = now()
WHERE id = (
  SELECT pending.id
  FROM bot_history_turns pending
  WHERE pending.session_id = sqlc.arg(session_id)
    AND pending.request_message_id IS NOT NULL
    AND pending.assistant_message_id IS NULL
    AND pending.superseded_at IS NULL
  ORDER BY pending.position DESC
  LIMIT 1
)
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: GetVisibleHistoryTurnByMessage :one
SELECT t.id, t.bot_id, t.session_id, t.position, t.request_message_id, t.assistant_message_id,
  t.superseded_by_turn_id, t.superseded_at, t.superseded_reason, t.created_at, t.updated_at
FROM bot_history_turns t
WHERE t.session_id = sqlc.arg(session_id)
  AND t.superseded_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM bot_visible_history_messages m
    WHERE m.turn_id = t.id
      AND m.id = sqlc.arg(message_id)
)
LIMIT 1;

-- name: GetLatestVisibleHistoryTurnBySession :one
SELECT id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at
FROM bot_history_turns
WHERE session_id = sqlc.arg(session_id)
  AND superseded_at IS NULL
ORDER BY position DESC
LIMIT 1;

-- name: GetHistoryTurnByID :one
SELECT id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at
FROM bot_history_turns
WHERE id = sqlc.arg(old_turn_id)
  AND session_id = sqlc.arg(session_id)
  AND superseded_at IS NULL
LIMIT 1;

-- name: ReplaceHistoryTurn :one
WITH old_turn AS (
  SELECT t.*
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(old_turn_id)
    AND t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
  FOR UPDATE
),
replacement AS (
  INSERT INTO bot_history_turns (
    bot_id,
    session_id,
    position,
    request_message_id,
    assistant_message_id
  )
  SELECT
    old_turn.bot_id,
    old_turn.session_id,
    COALESCE((
      SELECT MAX(position) + 1
      FROM bot_history_turns
      WHERE session_id = old_turn.session_id
    ), 1),
    sqlc.narg(request_message_id)::uuid,
    sqlc.narg(assistant_message_id)::uuid
  FROM old_turn
  RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
    superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at
),
updated AS (
  UPDATE bot_history_turns old
  SET superseded_by_turn_id = replacement.id,
      superseded_at = sqlc.arg(superseded_at),
      superseded_reason = sqlc.arg(superseded_reason),
      updated_at = now()
  FROM replacement
  WHERE old.id = sqlc.arg(old_turn_id)
    AND old.session_id = sqlc.arg(session_id)
    AND old.superseded_at IS NULL
  RETURNING old.id
)
SELECT replacement.id, replacement.bot_id, replacement.session_id, replacement.position,
  replacement.request_message_id, replacement.assistant_message_id,
  replacement.superseded_by_turn_id, replacement.superseded_at,
  replacement.superseded_reason, replacement.created_at, replacement.updated_at
FROM replacement
JOIN updated ON true;

-- name: SupersedeHistoryTurn :one
UPDATE bot_history_turns
SET superseded_by_turn_id = sqlc.arg(superseded_by_turn_id),
    superseded_at = sqlc.arg(superseded_at),
    superseded_reason = sqlc.arg(superseded_reason),
    updated_at = now()
WHERE id = sqlc.arg(old_turn_id)
  AND session_id = sqlc.arg(session_id)
  AND superseded_at IS NULL
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: ListMessages :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT 10000;

-- name: ListMessagesBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT 10000;

-- name: ListMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesSinceBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSinceBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeBySession :many
WITH candidate_turns AS (
  SELECT t.*
  FROM bot_history_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND COALESCE(req.created_at, assistant.created_at) < sqlc.arg(created_at)
  ORDER BY t.position DESC
  LIMIT sqlc.arg(max_count) + 2
),
session_turns AS (
  SELECT
    t.*,
    assistant.created_at AS assistant_created_at,
    assistant.id AS assistant_id,
    LEAD(COALESCE(req.created_at, assistant.created_at)) OVER (
      ORDER BY t.position
    ) AS next_created_at,
    LEAD(COALESCE(req.id, assistant.id)) OVER (
      ORDER BY t.position
    ) AS next_message_id
  FROM candidate_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
),
visible_messages AS (
  SELECT t.id AS turn_id, t.position AS turn_position, 1::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.request_message_id
  UNION ALL
  SELECT t.id AS turn_id, t.position AS turn_position, 2::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.assistant_message_id
  UNION ALL
  SELECT
    t.id AS turn_id,
    t.position AS turn_position,
    2 + ROW_NUMBER() OVER (PARTITION BY t.id ORDER BY m.created_at, m.id) AS turn_message_seq,
    m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m
    ON m.session_id = t.session_id
   AND m.role IN ('assistant', 'tool')
  WHERE t.assistant_message_id IS NOT NULL
    AND m.id <> t.assistant_message_id
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_turns anchored
      WHERE anchored.session_id = t.session_id
        AND (anchored.request_message_id = m.id OR anchored.assistant_message_id = m.id)
    )
    AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
    AND (
      t.next_created_at IS NULL
      OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
    )
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeMessageBySession :many
WITH cursor_turn AS (
  SELECT t.position
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND (t.request_message_id = sqlc.arg(before_message_id) OR t.assistant_message_id = sqlc.arg(before_message_id))
  LIMIT 1
),
candidate_turns AS (
  SELECT t.*
  FROM bot_history_turns t
  CROSS JOIN cursor_turn cursor
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND t.position <= cursor.position
  ORDER BY t.position DESC
  LIMIT sqlc.arg(max_count) + 2
),
session_turns AS (
  SELECT
    t.*,
    assistant.created_at AS assistant_created_at,
    assistant.id AS assistant_id,
    LEAD(COALESCE(req.created_at, assistant.created_at)) OVER (
      ORDER BY t.position
    ) AS next_created_at,
    LEAD(COALESCE(req.id, assistant.id)) OVER (
      ORDER BY t.position
    ) AS next_message_id
  FROM candidate_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
),
visible_messages AS (
  SELECT t.id AS turn_id, t.position AS turn_position, 1::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.request_message_id
  UNION ALL
  SELECT t.id AS turn_id, t.position AS turn_position, 2::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.assistant_message_id
  UNION ALL
  SELECT
    t.id AS turn_id,
    t.position AS turn_position,
    2 + ROW_NUMBER() OVER (PARTITION BY t.id ORDER BY m.created_at, m.id) AS turn_message_seq,
    m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m
    ON m.session_id = t.session_id
   AND m.role IN ('assistant', 'tool')
  WHERE t.assistant_message_id IS NOT NULL
    AND m.id <> t.assistant_message_id
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_turns anchored
      WHERE anchored.session_id = t.session_id
        AND (anchored.request_message_id = m.id OR anchored.assistant_message_id = m.id)
    )
    AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
    AND (
      t.next_created_at IS NULL
      OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
    )
),
cursor_message AS (
  SELECT
    t.position AS turn_position,
    CASE
      WHEN t.request_message_id = sqlc.arg(before_message_id) THEN 1::BIGINT
      WHEN t.assistant_message_id = sqlc.arg(before_message_id) THEN 2::BIGINT
      ELSE 2 + (
        SELECT COUNT(*)
        FROM bot_history_messages prior
        WHERE prior.session_id = t.session_id
          AND prior.role IN ('assistant', 'tool')
          AND prior.id <> t.assistant_message_id
          AND NOT EXISTS (
            SELECT 1
            FROM bot_history_turns anchored
            WHERE anchored.session_id = t.session_id
              AND (anchored.request_message_id = prior.id OR anchored.assistant_message_id = prior.id)
          )
          AND (prior.created_at, prior.id) > (t.assistant_created_at, t.assistant_id)
          AND (prior.created_at, prior.id) <= (m.created_at, m.id)
          AND (
            t.next_created_at IS NULL
            OR (prior.created_at, prior.id) < (t.next_created_at, t.next_message_id)
          )
      )
    END AS turn_message_seq,
    m.created_at,
    m.id
  FROM session_turns t
  JOIN bot_history_messages m
    ON m.id = sqlc.arg(before_message_id)
   AND (
    m.id = t.request_message_id
    OR m.id = t.assistant_message_id
    OR (
      t.assistant_message_id IS NOT NULL
      AND m.session_id = t.session_id
      AND m.role IN ('assistant', 'tool')
      AND m.id <> t.assistant_message_id
      AND NOT EXISTS (
        SELECT 1
        FROM bot_history_turns anchored
        WHERE anchored.session_id = t.session_id
          AND (anchored.request_message_id = m.id OR anchored.assistant_message_id = m.id)
      )
      AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
      AND (
        t.next_created_at IS NULL
        OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
      )
    )
   )
  LIMIT 1
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_messages m
CROSS JOIN cursor_message cursor
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (m.turn_position, m.turn_message_seq, m.created_at, m.id)
    < (cursor.turn_position, cursor.turn_message_seq, cursor.created_at, cursor.id)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatestBySession :many
WITH candidate_turns AS (
  SELECT t.*
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
  ORDER BY t.position DESC
  LIMIT sqlc.arg(max_count) + 2
),
session_turns AS (
  SELECT
    t.*,
    assistant.created_at AS assistant_created_at,
    assistant.id AS assistant_id,
    LEAD(COALESCE(req.created_at, assistant.created_at)) OVER (
      ORDER BY t.position
    ) AS next_created_at,
    LEAD(COALESCE(req.id, assistant.id)) OVER (
      ORDER BY t.position
    ) AS next_message_id
  FROM candidate_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
),
visible_messages AS (
  SELECT t.id AS turn_id, t.position AS turn_position, 1::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.request_message_id
  UNION ALL
  SELECT t.id AS turn_id, t.position AS turn_position, 2::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.assistant_message_id
  UNION ALL
  SELECT
    t.id AS turn_id,
    t.position AS turn_position,
    2 + ROW_NUMBER() OVER (PARTITION BY t.id ORDER BY m.created_at, m.id) AS turn_message_seq,
    m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m
    ON m.session_id = t.session_id
   AND m.role IN ('assistant', 'tool')
  WHERE t.assistant_message_id IS NOT NULL
    AND m.id <> t.assistant_message_id
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_turns anchored
      WHERE anchored.session_id = t.session_id
        AND (anchored.request_message_id = m.id OR anchored.assistant_message_id = m.id)
    )
    AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
    AND (
      t.next_created_at IS NULL
      OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
    )
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: GetMessageByIDBySession :one
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t
  ON t.session_id = m.session_id
 AND t.superseded_at IS NULL
 AND (t.request_message_id = m.id OR t.assistant_message_id = m.id)
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.id = sqlc.arg(message_id)
LIMIT 1;

-- name: GetMessageByExternalIDBySession :one
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t
  ON t.session_id = m.session_id
 AND t.superseded_at IS NULL
 AND (t.request_message_id = m.id OR t.assistant_message_id = m.id)
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY t.position DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: ListMessagesAfterBySession :many
WITH candidate_turns AS (
  SELECT t.*
  FROM bot_history_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND COALESCE(req.created_at, assistant.created_at) > sqlc.arg(created_at)
  ORDER BY t.position ASC
  LIMIT sqlc.arg(max_count) + 2
),
session_turns AS (
  SELECT
    t.*,
    assistant.created_at AS assistant_created_at,
    assistant.id AS assistant_id,
    LEAD(COALESCE(req.created_at, assistant.created_at)) OVER (
      ORDER BY t.position
    ) AS next_created_at,
    LEAD(COALESCE(req.id, assistant.id)) OVER (
      ORDER BY t.position
    ) AS next_message_id
  FROM candidate_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
),
visible_messages AS (
  SELECT t.id AS turn_id, t.position AS turn_position, 1::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.request_message_id
  UNION ALL
  SELECT t.id AS turn_id, t.position AS turn_position, 2::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.assistant_message_id
  UNION ALL
  SELECT
    t.id AS turn_id,
    t.position AS turn_position,
    2 + ROW_NUMBER() OVER (PARTITION BY t.id ORDER BY m.created_at, m.id) AS turn_message_seq,
    m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m
    ON m.session_id = t.session_id
   AND m.role IN ('assistant', 'tool')
  WHERE t.assistant_message_id IS NOT NULL
    AND m.id <> t.assistant_message_id
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_turns anchored
      WHERE anchored.session_id = t.session_id
        AND (anchored.request_message_id = m.id OR anchored.assistant_message_id = m.id)
    )
    AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
    AND (
      t.next_created_at IS NULL
      OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
    )
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at > sqlc.arg(created_at)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesAfterMessageBySession :many
WITH cursor_turn AS (
  SELECT t.position
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND (t.request_message_id = sqlc.arg(after_message_id) OR t.assistant_message_id = sqlc.arg(after_message_id))
  LIMIT 1
),
candidate_turns AS (
  SELECT t.*
  FROM bot_history_turns t
  CROSS JOIN cursor_turn cursor
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND t.position >= cursor.position
  ORDER BY t.position ASC
  LIMIT sqlc.arg(max_count) + 2
),
session_turns AS (
  SELECT
    t.*,
    assistant.created_at AS assistant_created_at,
    assistant.id AS assistant_id,
    LEAD(COALESCE(req.created_at, assistant.created_at)) OVER (
      ORDER BY t.position
    ) AS next_created_at,
    LEAD(COALESCE(req.id, assistant.id)) OVER (
      ORDER BY t.position
    ) AS next_message_id
  FROM candidate_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
),
visible_messages AS (
  SELECT t.id AS turn_id, t.position AS turn_position, 1::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.request_message_id
  UNION ALL
  SELECT t.id AS turn_id, t.position AS turn_position, 2::BIGINT AS turn_message_seq, m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m ON m.id = t.assistant_message_id
  UNION ALL
  SELECT
    t.id AS turn_id,
    t.position AS turn_position,
    2 + ROW_NUMBER() OVER (PARTITION BY t.id ORDER BY m.created_at, m.id) AS turn_message_seq,
    m.id, m.bot_id, m.session_id, m.sender_channel_identity_id, m.sender_account_user_id, m.source_message_id, m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage, m.session_mode, m.runtime_type, m.event_id, m.display_text, m.created_at
  FROM session_turns t
  JOIN bot_history_messages m
    ON m.session_id = t.session_id
   AND m.role IN ('assistant', 'tool')
  WHERE t.assistant_message_id IS NOT NULL
    AND m.id <> t.assistant_message_id
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_turns anchored
      WHERE anchored.session_id = t.session_id
        AND (anchored.request_message_id = m.id OR anchored.assistant_message_id = m.id)
    )
    AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
    AND (
      t.next_created_at IS NULL
      OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
    )
),
cursor_message AS (
  SELECT
    t.position AS turn_position,
    CASE
      WHEN t.request_message_id = sqlc.arg(after_message_id) THEN 1::BIGINT
      WHEN t.assistant_message_id = sqlc.arg(after_message_id) THEN 2::BIGINT
      ELSE 2 + (
        SELECT COUNT(*)
        FROM bot_history_messages prior
        WHERE prior.session_id = t.session_id
          AND prior.role IN ('assistant', 'tool')
          AND prior.id <> t.assistant_message_id
          AND NOT EXISTS (
            SELECT 1
            FROM bot_history_turns anchored
            WHERE anchored.session_id = t.session_id
              AND (anchored.request_message_id = prior.id OR anchored.assistant_message_id = prior.id)
          )
          AND (prior.created_at, prior.id) > (t.assistant_created_at, t.assistant_id)
          AND (prior.created_at, prior.id) <= (m.created_at, m.id)
          AND (
            t.next_created_at IS NULL
            OR (prior.created_at, prior.id) < (t.next_created_at, t.next_message_id)
          )
      )
    END AS turn_message_seq,
    m.created_at,
    m.id
  FROM session_turns t
  JOIN bot_history_messages m
    ON m.id = sqlc.arg(after_message_id)
   AND (
    m.id = t.request_message_id
    OR m.id = t.assistant_message_id
    OR (
      t.assistant_message_id IS NOT NULL
      AND m.session_id = t.session_id
      AND m.role IN ('assistant', 'tool')
      AND m.id <> t.assistant_message_id
      AND NOT EXISTS (
        SELECT 1
        FROM bot_history_turns anchored
        WHERE anchored.session_id = t.session_id
          AND (anchored.request_message_id = m.id OR anchored.assistant_message_id = m.id)
      )
      AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
      AND (
        t.next_created_at IS NULL
        OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
      )
    )
   )
  LIMIT 1
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_messages m
CROSS JOIN cursor_message cursor
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (m.turn_position, m.turn_message_seq, m.created_at, m.id)
    > (cursor.turn_position, cursor.turn_message_seq, cursor.created_at, cursor.id)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListVisibleMessagesFromBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_position >= (
    SELECT target.turn_position
    FROM bot_visible_history_messages target
    WHERE target.session_id = sqlc.arg(session_id)
      AND target.id = sqlc.arg(message_id)
    LIMIT 1
  )
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: CountMessagesByBot :one
SELECT COUNT(*) FROM bot_visible_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesByBot :exec
DELETE FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesBySession :exec
DELETE FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id);

-- name: DeleteMessagesByIDs :exec
DELETE FROM bot_history_messages
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListObservedConversationsByChannelIdentity :many
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at)::timestamptz AS last_observed_at
  FROM bot_visible_history_messages m
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
  FROM bot_visible_history_messages m
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
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(session_id)::uuid IS NULL OR m.session_id = sqlc.narg(session_id)::uuid)
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
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: MarkMessagesCompacted :exec
UPDATE bot_history_messages
SET compact_id = $1
WHERE id = ANY($2::uuid[]);

-- name: ListUncompactedMessagesBySession :many
SELECT id, bot_id, session_id, role, content, usage, sender_channel_identity_id, compact_id, created_at
FROM bot_visible_history_messages
WHERE session_id = $1
  AND compact_id IS NULL
ORDER BY turn_position ASC, turn_message_seq ASC, created_at ASC, id ASC;
