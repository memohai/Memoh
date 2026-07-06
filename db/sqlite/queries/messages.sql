-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  id, bot_id, session_id, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, session_mode, runtime_type, model_id, event_id, display_text, created_at
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
  sqlc.arg(session_mode),
  sqlc.arg(runtime_type),
  sqlc.narg(model_id),
  sqlc.narg(event_id),
  sqlc.narg(display_text),
  strftime(
    '%Y-%m-%d %H:%M:%f',
    max(
      julianday('now'),
      COALESCE((
        SELECT MAX(julianday(created_at)) + (1.0 / 86400000.0)
        FROM bot_history_messages
        WHERE session_id = sqlc.narg(session_id)
      ), julianday('now'))
    )
  )
)
RETURNING
  id, bot_id, session_id, sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id, role, content, metadata, usage, session_mode, runtime_type,
  event_id, display_text, created_at;

-- name: CreateMessageWithTurn :one
INSERT INTO bot_history_messages (
  id, bot_id, session_id, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, session_mode, runtime_type, model_id, event_id, display_text,
  turn_id, turn_position, turn_message_seq, turn_visible, created_at
)
VALUES (
  sqlc.arg(message_id),
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
  sqlc.arg(session_mode),
  sqlc.arg(runtime_type),
  sqlc.narg(model_id),
  sqlc.narg(event_id),
  sqlc.narg(display_text),
  sqlc.arg(turn_id),
  COALESCE(CAST(sqlc.narg(turn_position) AS INTEGER), (
    SELECT existing.turn_position
    FROM bot_history_messages existing
    WHERE existing.turn_id = sqlc.arg(turn_id)
      AND session_id = sqlc.narg(session_id)
      AND existing.turn_position IS NOT NULL
    LIMIT 1
  )),
  sqlc.arg(turn_message_seq),
  CASE
    WHEN CAST(sqlc.narg(turn_position) AS INTEGER) IS NOT NULL THEN 1
    ELSE COALESCE((
      SELECT CASE WHEN existing.turn_visible = 1 THEN 1 ELSE 0 END
      FROM bot_history_messages existing
      WHERE existing.turn_id = sqlc.arg(turn_id)
        AND existing.session_id = sqlc.narg(session_id)
      LIMIT 1
    ), 0)
  END,
  strftime(
    '%Y-%m-%d %H:%M:%f',
    max(
      julianday('now'),
      COALESCE((
        SELECT MAX(julianday(created_at)) + (1.0 / 86400000.0)
        FROM bot_history_messages
        WHERE session_id = sqlc.narg(session_id)
      ), julianday('now'))
    )
  )
)
RETURNING
  id, bot_id, session_id, sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id, role, content, metadata, usage, session_mode, runtime_type,
  event_id, display_text, created_at;

-- name: AllocateSessionTurnPosition :one
UPDATE bot_sessions
SET next_turn_position = next_turn_position + 1
WHERE id = sqlc.arg(session_id)
RETURNING next_turn_position - 1 AS position;

-- name: CreateMessageInHistoryTurnByRequest :one
INSERT INTO bot_history_messages (
  id, bot_id, session_id, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, session_mode, runtime_type, model_id, event_id, display_text,
  turn_id, turn_position, turn_message_seq, turn_visible, created_at
)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  target.session_id,
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
  sqlc.narg(display_text),
  target.turn_id,
  target.turn_position,
  CASE
    WHEN sqlc.arg(role) = 'assistant' AND NOT EXISTS (
      SELECT 1
      FROM bot_history_messages assistant
      WHERE assistant.turn_id = target.turn_id
        AND assistant.role = 'assistant'
        AND assistant.turn_message_seq = 2
        AND assistant.turn_visible = 1
    ) THEN 2
    ELSE COALESCE((
      SELECT existing.turn_message_seq + 1
      FROM bot_history_messages existing
      WHERE existing.turn_id = target.turn_id
      ORDER BY existing.turn_message_seq DESC
      LIMIT 1
    ), 1)
  END,
  1,
  strftime(
    '%Y-%m-%d %H:%M:%f',
    max(
      julianday('now'),
      COALESCE((
        SELECT MAX(julianday(created_at)) + (1.0 / 86400000.0)
        FROM bot_history_messages
        WHERE session_id = target.session_id
      ), julianday('now'))
    )
  )
FROM bot_history_messages target
WHERE target.session_id = sqlc.arg(session_id)
  AND target.id = sqlc.arg(request_message_id)
  AND target.turn_id IS NOT NULL
  AND target.turn_position IS NOT NULL
  AND target.turn_visible = 1
LIMIT 1
RETURNING
  id, bot_id, session_id, sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id, role, content, metadata, usage, session_mode, runtime_type,
  event_id, display_text, created_at;

-- name: CreateHistoryTurn :one
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))) AS id,
  s.bot_id,
  s.id AS session_id,
  CAST(sqlc.arg(turn_position) AS INTEGER) AS position,
  CAST(sqlc.narg(request_message_id) AS TEXT) AS request_message_id,
  CAST(sqlc.narg(assistant_message_id) AS TEXT) AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  CURRENT_TIMESTAMP AS created_at,
  CURRENT_TIMESTAMP AS updated_at
FROM bot_sessions s
WHERE s.id = sqlc.arg(session_id)
  AND s.bot_id = sqlc.arg(bot_id);

-- name: CreateHistoryTurnWithID :one
SELECT
  sqlc.arg(turn_id) AS id,
  s.bot_id,
  s.id AS session_id,
  CAST(sqlc.arg(turn_position) AS INTEGER) AS position,
  CAST(sqlc.narg(request_message_id) AS TEXT) AS request_message_id,
  CAST(sqlc.narg(assistant_message_id) AS TEXT) AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  CURRENT_TIMESTAMP AS created_at,
  CURRENT_TIMESTAMP AS updated_at
FROM bot_sessions s
WHERE s.id = sqlc.arg(session_id)
  AND s.bot_id = sqlc.arg(bot_id);

-- name: CreateHistoryTurnWithIDAtPosition :one
SELECT
  sqlc.arg(turn_id) AS id,
  s.bot_id,
  s.id AS session_id,
  CAST(sqlc.arg(turn_position) AS INTEGER) AS position,
  CAST(sqlc.narg(request_message_id) AS TEXT) AS request_message_id,
  CAST(sqlc.narg(assistant_message_id) AS TEXT) AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  CURRENT_TIMESTAMP AS created_at,
  CURRENT_TIMESTAMP AS updated_at
FROM bot_sessions s
WHERE s.id = sqlc.arg(session_id)
  AND s.bot_id = sqlc.arg(bot_id);

-- name: BindHistoryTurnAssistantByRequest :one
SELECT
  request.turn_id AS id,
  request.bot_id,
  request.session_id,
  request.turn_position AS position,
  request.id AS request_message_id,
  assistant.id AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  request.created_at AS created_at,
  CURRENT_TIMESTAMP AS updated_at
FROM bot_history_messages request
JOIN bot_history_messages assistant ON assistant.id = sqlc.arg(assistant_message_id)
  AND assistant.session_id = request.session_id
WHERE request.session_id = sqlc.arg(session_id)
  AND request.id = sqlc.arg(request_message_id)
  AND request.turn_id IS NOT NULL
  AND request.turn_position IS NOT NULL
  AND request.turn_visible = 1
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_messages assistant
    WHERE assistant.turn_id = request.turn_id
      AND assistant.role = 'assistant'
      AND assistant.turn_message_seq = 2
      AND assistant.turn_visible = 1
  )
LIMIT 1;

-- name: BindLatestHistoryTurnAssistant :one
SELECT
  request.turn_id AS id,
  request.bot_id,
  request.session_id,
  request.turn_position AS position,
  request.id AS request_message_id,
  assistant.id AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  request.created_at AS created_at,
  CURRENT_TIMESTAMP AS updated_at
FROM bot_history_messages request
JOIN bot_history_messages assistant ON assistant.id = sqlc.arg(assistant_message_id)
  AND assistant.session_id = request.session_id
WHERE request.session_id = sqlc.arg(session_id)
  AND request.role = 'user'
  AND request.turn_message_seq = 1
  AND request.turn_id IS NOT NULL
  AND request.turn_position IS NOT NULL
  AND request.turn_visible = 1
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_messages assistant
    WHERE assistant.turn_id = request.turn_id
      AND assistant.role = 'assistant'
      AND assistant.turn_message_seq = 2
      AND assistant.turn_visible = 1
  )
ORDER BY request.turn_position DESC
LIMIT 1;

-- name: LinkMessageToHistoryTurn :one
UPDATE bot_history_messages
SET turn_id = sqlc.arg(turn_id),
    turn_position = (
      SELECT existing.turn_position
      FROM bot_history_messages existing
      WHERE existing.turn_id = sqlc.arg(turn_id)
        AND existing.session_id = bot_history_messages.session_id
        AND existing.turn_position IS NOT NULL
      LIMIT 1
    ),
    turn_visible = COALESCE((
      SELECT CASE WHEN existing.turn_visible = 1 THEN 1 ELSE 0 END
      FROM bot_history_messages existing
      WHERE existing.turn_id = sqlc.arg(turn_id)
        AND existing.session_id = bot_history_messages.session_id
      LIMIT 1
    ), 0),
    turn_message_seq = sqlc.arg(turn_message_seq),
    turn_superseded_by_turn_id = NULL,
    turn_superseded_at = NULL,
    turn_superseded_reason = NULL
WHERE bot_history_messages.id = sqlc.arg(message_id)
  AND EXISTS (
    SELECT 1
    FROM bot_history_messages existing
    WHERE existing.turn_id = sqlc.arg(turn_id)
      AND existing.session_id = bot_history_messages.session_id
      AND existing.turn_position IS NOT NULL
  )
RETURNING id;

-- name: AppendMessageToHistoryTurnByRequest :one
UPDATE bot_history_messages
SET turn_id = (
      SELECT request.turn_id
      FROM bot_history_messages request
      WHERE request.session_id = sqlc.arg(session_id)
        AND request.id = sqlc.arg(request_message_id)
        AND request.turn_id IS NOT NULL
        AND request.turn_visible = 1
      LIMIT 1
    ),
    turn_position = (
      SELECT request.turn_position
      FROM bot_history_messages request
      WHERE request.session_id = sqlc.arg(session_id)
        AND request.id = sqlc.arg(request_message_id)
        AND request.turn_position IS NOT NULL
        AND request.turn_visible = 1
      LIMIT 1
    ),
    turn_visible = 1,
    turn_message_seq = COALESCE((
      SELECT existing.turn_message_seq + 1
      FROM bot_history_messages existing
      WHERE existing.turn_id = (
        SELECT request.turn_id
        FROM bot_history_messages request
        WHERE request.session_id = sqlc.arg(session_id)
          AND request.id = sqlc.arg(request_message_id)
          AND request.turn_id IS NOT NULL
          AND request.turn_visible = 1
        LIMIT 1
      )
      ORDER BY existing.turn_message_seq DESC
      LIMIT 1
    ), 1)
WHERE bot_history_messages.id = sqlc.arg(message_id)
  AND bot_history_messages.session_id = sqlc.arg(session_id)
  AND bot_history_messages.turn_id IS NULL
  AND EXISTS (
    SELECT 1
    FROM bot_history_messages request
    WHERE request.session_id = sqlc.arg(session_id)
      AND request.id = sqlc.arg(request_message_id)
      AND request.turn_id IS NOT NULL
      AND request.turn_visible = 1
  )
RETURNING id;

-- name: AppendMessageToLatestHistoryTurn :exec
UPDATE bot_history_messages
SET turn_id = (
      SELECT latest.turn_id
      FROM bot_history_messages latest
      WHERE latest.session_id = sqlc.arg(session_id)
        AND latest.turn_id IS NOT NULL
        AND latest.turn_position IS NOT NULL
        AND latest.turn_visible = 1
      ORDER BY latest.turn_position DESC, latest.turn_message_seq DESC
      LIMIT 1
    ),
    turn_position = (
      SELECT latest.turn_position
      FROM bot_history_messages latest
      WHERE latest.session_id = sqlc.arg(session_id)
        AND latest.turn_id IS NOT NULL
        AND latest.turn_position IS NOT NULL
        AND latest.turn_visible = 1
      ORDER BY latest.turn_position DESC, latest.turn_message_seq DESC
      LIMIT 1
    ),
    turn_visible = CASE
      WHEN EXISTS (
        SELECT 1
        FROM bot_history_messages latest
        WHERE latest.session_id = sqlc.arg(session_id)
          AND latest.turn_id IS NOT NULL
          AND latest.turn_visible = 1
      ) THEN 1
      ELSE 0
    END,
    turn_message_seq = COALESCE((
      SELECT existing.turn_message_seq + 1
      FROM bot_history_messages existing
      WHERE existing.turn_id = (
        SELECT latest.turn_id
        FROM bot_history_messages latest
        WHERE latest.session_id = sqlc.arg(session_id)
          AND latest.turn_id IS NOT NULL
          AND latest.turn_position IS NOT NULL
          AND latest.turn_visible = 1
        ORDER BY latest.turn_position DESC, latest.turn_message_seq DESC
        LIMIT 1
      )
      ORDER BY existing.turn_message_seq DESC
      LIMIT 1
    ), 1)
WHERE bot_history_messages.id = sqlc.arg(message_id)
  AND bot_history_messages.session_id = sqlc.arg(session_id)
  AND bot_history_messages.turn_id IS NULL;

-- name: LinkUnassignedMessagesAfterHistoryTurnAssistant :exec
UPDATE bot_history_messages
SET turn_id = (
      SELECT assistant.turn_id
      FROM bot_history_messages assistant
      WHERE assistant.turn_id = sqlc.arg(turn_id)
        AND assistant.role = 'assistant'
        AND assistant.turn_message_seq = 2
        AND assistant.turn_visible = 1
      LIMIT 1
    ),
    turn_position = (
      SELECT assistant.turn_position
      FROM bot_history_messages assistant
      WHERE assistant.turn_id = sqlc.arg(turn_id)
        AND assistant.role = 'assistant'
        AND assistant.turn_message_seq = 2
        AND assistant.turn_visible = 1
      LIMIT 1
    ),
    turn_visible = 1,
    turn_superseded_by_turn_id = NULL,
    turn_superseded_at = NULL,
    turn_superseded_reason = NULL,
    turn_message_seq = (
      SELECT 2 + COUNT(*)
      FROM bot_history_messages prior
      JOIN bot_history_messages assistant ON assistant.turn_id = sqlc.arg(turn_id)
        AND assistant.role = 'assistant'
        AND assistant.turn_message_seq = 2
        AND assistant.turn_visible = 1
      WHERE prior.session_id = assistant.session_id
        AND prior.role IN ('assistant', 'tool')
        AND prior.id <> assistant.id
        AND prior.turn_id IS NULL
        AND (
          prior.created_at < bot_history_messages.created_at
          OR (prior.created_at = bot_history_messages.created_at AND prior.rowid <= bot_history_messages.rowid)
        )
        AND (
          prior.created_at > assistant.created_at
          OR (prior.created_at = assistant.created_at AND prior.rowid > assistant.rowid)
        )
    )
WHERE bot_history_messages.turn_id IS NULL
  AND bot_history_messages.role IN ('assistant', 'tool')
  AND EXISTS (
    SELECT 1
    FROM bot_history_messages assistant
    WHERE assistant.turn_id = sqlc.arg(turn_id)
      AND assistant.role = 'assistant'
      AND assistant.turn_message_seq = 2
      AND assistant.turn_visible = 1
      AND bot_history_messages.session_id = assistant.session_id
      AND bot_history_messages.id <> assistant.id
      AND (
        bot_history_messages.created_at > assistant.created_at
        OR (bot_history_messages.created_at = assistant.created_at AND bot_history_messages.rowid > assistant.rowid)
      )
  );

-- name: GetVisibleHistoryTurnByMessage :one
WITH target AS (
  SELECT m.turn_id, m.session_id, m.turn_position
  FROM bot_history_messages m
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.id = sqlc.arg(message_id)
    AND m.turn_id IS NOT NULL
    AND m.turn_position IS NOT NULL
    AND m.turn_visible = 1
  LIMIT 1
)
SELECT
  target.turn_id AS id,
  (
    SELECT first_message.bot_id
    FROM bot_history_messages first_message
    WHERE first_message.turn_id = target.turn_id
      AND first_message.session_id = target.session_id
    ORDER BY first_message.turn_message_seq ASC, first_message.created_at ASC, first_message.id ASC
    LIMIT 1
  ) AS bot_id,
  target.session_id,
  target.turn_position AS position,
  COALESCE((
    SELECT request_message.id
    FROM bot_history_messages request_message
    WHERE request_message.turn_id = target.turn_id
      AND request_message.session_id = target.session_id
      AND request_message.role = 'user'
      AND request_message.turn_message_seq = 1
    ORDER BY request_message.created_at ASC, request_message.id ASC
    LIMIT 1
  ), '') AS request_message_id,
  COALESCE((
    SELECT assistant_message.id
    FROM bot_history_messages assistant_message
    WHERE assistant_message.turn_id = target.turn_id
      AND assistant_message.session_id = target.session_id
      AND assistant_message.role = 'assistant'
      AND assistant_message.turn_message_seq = 2
    ORDER BY assistant_message.created_at ASC, assistant_message.id ASC
    LIMIT 1
  ), '') AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  MIN(m.created_at) AS created_at,
  MAX(m.created_at) AS updated_at
FROM target
JOIN bot_history_messages m ON m.turn_id = target.turn_id
  AND m.session_id = target.session_id
GROUP BY target.turn_id, target.session_id, target.turn_position;

-- name: GetLatestVisibleHistoryTurnBySession :one
WITH target AS (
  SELECT m.turn_id, m.session_id, m.turn_position
  FROM bot_history_messages m
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.turn_id IS NOT NULL
    AND m.turn_position IS NOT NULL
    AND m.turn_visible = 1
  ORDER BY m.turn_position DESC, m.turn_message_seq DESC
  LIMIT 1
)
SELECT
  target.turn_id AS id,
  (
    SELECT first_message.bot_id
    FROM bot_history_messages first_message
    WHERE first_message.turn_id = target.turn_id
      AND first_message.session_id = target.session_id
    ORDER BY first_message.turn_message_seq ASC, first_message.created_at ASC, first_message.id ASC
    LIMIT 1
  ) AS bot_id,
  target.session_id,
  target.turn_position AS position,
  COALESCE((
    SELECT request_message.id
    FROM bot_history_messages request_message
    WHERE request_message.turn_id = target.turn_id
      AND request_message.session_id = target.session_id
      AND request_message.role = 'user'
      AND request_message.turn_message_seq = 1
    ORDER BY request_message.created_at ASC, request_message.id ASC
    LIMIT 1
  ), '') AS request_message_id,
  COALESCE((
    SELECT assistant_message.id
    FROM bot_history_messages assistant_message
    WHERE assistant_message.turn_id = target.turn_id
      AND assistant_message.session_id = target.session_id
      AND assistant_message.role = 'assistant'
      AND assistant_message.turn_message_seq = 2
    ORDER BY assistant_message.created_at ASC, assistant_message.id ASC
    LIMIT 1
  ), '') AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  MIN(m.created_at) AS created_at,
  MAX(m.created_at) AS updated_at
FROM target
JOIN bot_history_messages m ON m.turn_id = target.turn_id
  AND m.session_id = target.session_id
GROUP BY target.turn_id, target.session_id, target.turn_position;

-- name: GetHistoryTurnByID :one
WITH target AS (
  SELECT m.turn_id, m.session_id, m.turn_position
  FROM bot_history_messages m
  WHERE m.turn_id = sqlc.arg(old_turn_id)
    AND m.session_id = sqlc.arg(session_id)
    AND m.turn_position IS NOT NULL
    AND m.turn_visible = 1
  LIMIT 1
)
SELECT
  target.turn_id AS id,
  (
    SELECT first_message.bot_id
    FROM bot_history_messages first_message
    WHERE first_message.turn_id = target.turn_id
      AND first_message.session_id = target.session_id
    ORDER BY first_message.turn_message_seq ASC, first_message.created_at ASC, first_message.id ASC
    LIMIT 1
  ) AS bot_id,
  target.session_id,
  target.turn_position AS position,
  COALESCE((
    SELECT request_message.id
    FROM bot_history_messages request_message
    WHERE request_message.turn_id = target.turn_id
      AND request_message.session_id = target.session_id
      AND request_message.role = 'user'
      AND request_message.turn_message_seq = 1
    ORDER BY request_message.created_at ASC, request_message.id ASC
    LIMIT 1
  ), '') AS request_message_id,
  COALESCE((
    SELECT assistant_message.id
    FROM bot_history_messages assistant_message
    WHERE assistant_message.turn_id = target.turn_id
      AND assistant_message.session_id = target.session_id
      AND assistant_message.role = 'assistant'
      AND assistant_message.turn_message_seq = 2
    ORDER BY assistant_message.created_at ASC, assistant_message.id ASC
    LIMIT 1
  ), '') AS assistant_message_id,
  NULL AS superseded_by_turn_id,
  NULL AS superseded_at,
  NULL AS superseded_reason,
  MIN(m.created_at) AS created_at,
  MAX(m.created_at) AS updated_at
FROM target
JOIN bot_history_messages m ON m.turn_id = target.turn_id
  AND m.session_id = target.session_id
GROUP BY target.turn_id, target.session_id, target.turn_position;

-- name: SupersedeHistoryTurn :one
UPDATE bot_history_messages
SET turn_superseded_by_turn_id = sqlc.arg(superseded_by_turn_id),
    turn_superseded_at = sqlc.arg(superseded_at),
    turn_superseded_reason = sqlc.arg(superseded_reason),
    turn_visible = 0
WHERE turn_id = sqlc.arg(old_turn_id)
  AND session_id = sqlc.arg(session_id)
  AND turn_superseded_at IS NULL
RETURNING
  turn_id AS id,
  bot_id,
  session_id,
  turn_position AS position,
  CASE WHEN role = 'user' AND turn_message_seq = 1 THEN id ELSE '' END AS request_message_id,
  CASE WHEN role = 'assistant' AND turn_message_seq = 2 THEN id ELSE '' END AS assistant_message_id,
  turn_superseded_by_turn_id AS superseded_by_turn_id,
  turn_superseded_at AS superseded_at,
  turn_superseded_reason AS superseded_reason,
  created_at,
  COALESCE(turn_superseded_at, created_at) AS updated_at;

-- name: HideMessagesByHistoryTurn :exec
UPDATE bot_history_messages
SET turn_visible = 0
WHERE turn_id = sqlc.arg(turn_id);

-- name: ListMessages :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at ASC, m.id ASC
LIMIT 10000;

-- name: ListAllMessagesForBackup :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.turn_visible,
  m.turn_superseded_by_turn_id,
  m.turn_superseded_at,
  m.turn_superseded_reason,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at ASC, m.id ASC;

-- name: ListHistoryTurnsByBot :many
SELECT
  m.turn_id AS id,
  (
    SELECT first_message.bot_id
    FROM bot_history_messages first_message
    WHERE first_message.turn_id = m.turn_id
      AND first_message.session_id = m.session_id
    ORDER BY first_message.turn_message_seq ASC, first_message.created_at ASC, first_message.id ASC
    LIMIT 1
  ) AS bot_id,
  m.session_id,
  m.turn_position AS position,
  COALESCE((
    SELECT request_message.id
    FROM bot_history_messages request_message
    WHERE request_message.turn_id = m.turn_id
      AND request_message.session_id = m.session_id
      AND request_message.role = 'user'
      AND request_message.turn_message_seq = 1
    ORDER BY request_message.created_at ASC, request_message.id ASC
    LIMIT 1
  ), '') AS request_message_id,
  COALESCE((
    SELECT assistant_message.id
    FROM bot_history_messages assistant_message
    WHERE assistant_message.turn_id = m.turn_id
      AND assistant_message.session_id = m.session_id
      AND assistant_message.role = 'assistant'
      AND assistant_message.turn_message_seq = 2
    ORDER BY assistant_message.created_at ASC, assistant_message.id ASC
    LIMIT 1
  ), '') AS assistant_message_id,
  (
    SELECT superseded_message.turn_superseded_by_turn_id
    FROM bot_history_messages superseded_message
    WHERE superseded_message.turn_id = m.turn_id
      AND superseded_message.session_id = m.session_id
      AND superseded_message.turn_superseded_by_turn_id IS NOT NULL
    ORDER BY superseded_message.turn_superseded_at DESC, superseded_message.created_at DESC, superseded_message.id DESC
    LIMIT 1
  ) AS superseded_by_turn_id,
  MAX(m.turn_superseded_at) AS superseded_at,
  (
    SELECT superseded_message.turn_superseded_reason
    FROM bot_history_messages superseded_message
    WHERE superseded_message.turn_id = m.turn_id
      AND superseded_message.session_id = m.session_id
      AND superseded_message.turn_superseded_reason IS NOT NULL
    ORDER BY superseded_message.turn_superseded_at DESC, superseded_message.created_at DESC, superseded_message.id DESC
    LIMIT 1
  ) AS superseded_reason,
  MIN(m.created_at) AS created_at,
  COALESCE(MAX(m.turn_superseded_at), MAX(m.created_at)) AS updated_at
FROM bot_history_messages m
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.session_id IS NOT NULL
GROUP BY m.turn_id, m.session_id, m.turn_position
ORDER BY m.session_id ASC, m.turn_position ASC, m.turn_id ASC;

-- name: ListMessagesBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
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
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND julianday(m.created_at) >= julianday(sqlc.arg(created_at))
ORDER BY m.created_at ASC, m.id ASC;

-- name: ListMessagesSinceBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND julianday(m.created_at) >= julianday(sqlc.arg(created_at))
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND julianday(m.created_at) >= julianday(sqlc.arg(created_at))
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSinceBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND julianday(m.created_at) >= julianday(sqlc.arg(created_at))
  AND (json_extract(m.metadata, '$.trigger_mode') IS NULL OR json_extract(m.metadata, '$.trigger_mode') != 'passive_sync')
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND julianday(m.created_at) < julianday(sqlc.arg(created_at))
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND julianday(m.created_at) < julianday(sqlc.arg(created_at))
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeMessageBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND (m.turn_position, m.turn_message_seq, julianday(m.created_at), m.id)
    < (
      SELECT c.turn_position, c.turn_message_seq, julianday(c.created_at), c.id
      FROM bot_history_messages c
      WHERE c.session_id = sqlc.arg(session_id)
        AND c.turn_visible = 1
        AND c.turn_id IS NOT NULL
        AND c.turn_position IS NOT NULL
        AND c.turn_message_seq IS NOT NULL
        AND c.id = sqlc.arg(before_message_id)
      LIMIT 1
  )
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatestBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatestUIBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata,
  m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: GetMessageByIDBySession :one
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.id = sqlc.arg(message_id)
LIMIT 1;

-- name: GetMessageByExternalIDBySession :one
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: GetVisibleMessageCursorByExternalIDBySession :one
SELECT
  m.id,
  m.turn_position,
  m.turn_message_seq,
  m.created_at
FROM bot_history_messages m
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: GetVisibleMessageCursorByIDBySession :one
SELECT
  m.id,
  m.turn_position,
  m.turn_message_seq,
  m.created_at
FROM bot_history_messages m
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND m.id = sqlc.arg(message_id)
LIMIT 1;

-- name: GetLocatedMessageByExternalIDBySession :one
SELECT
  m.id,
  m.turn_position,
  m.turn_message_seq,
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
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: LocateMessagesWindowByExternalIDBySession :many
WITH target AS (
  SELECT
    m.id,
    m.turn_position,
    m.turn_message_seq,
    m.created_at
  FROM bot_history_messages m
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.turn_visible = 1
    AND m.turn_id IS NOT NULL
    AND m.turn_position IS NOT NULL
    AND m.turn_message_seq IS NOT NULL
    AND m.source_message_id = sqlc.arg(external_message_id)
  ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
  LIMIT 1
),
before_rows AS (
  SELECT m.id
  FROM bot_history_messages m
  CROSS JOIN target
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.turn_visible = 1
    AND m.turn_id IS NOT NULL
    AND m.turn_position IS NOT NULL
    AND m.turn_message_seq IS NOT NULL
    AND (
      m.turn_position < target.turn_position
      OR (m.turn_position = target.turn_position AND m.turn_message_seq < target.turn_message_seq)
      OR (m.turn_position = target.turn_position AND m.turn_message_seq = target.turn_message_seq AND julianday(m.created_at) < julianday(target.created_at))
      OR (m.turn_position = target.turn_position AND m.turn_message_seq = target.turn_message_seq AND julianday(m.created_at) = julianday(target.created_at) AND m.id < target.id)
    )
  ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
  LIMIT sqlc.arg(before_limit)
),
after_rows AS (
  SELECT m.id
  FROM bot_history_messages m
  CROSS JOIN target
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.turn_visible = 1
    AND m.turn_id IS NOT NULL
    AND m.turn_position IS NOT NULL
    AND m.turn_message_seq IS NOT NULL
    AND (
      m.turn_position > target.turn_position
      OR (m.turn_position = target.turn_position AND m.turn_message_seq > target.turn_message_seq)
      OR (m.turn_position = target.turn_position AND m.turn_message_seq = target.turn_message_seq AND julianday(m.created_at) > julianday(target.created_at))
      OR (m.turn_position = target.turn_position AND m.turn_message_seq = target.turn_message_seq AND julianday(m.created_at) = julianday(target.created_at) AND m.id > target.id)
    )
  ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
  LIMIT sqlc.arg(after_limit)
),
window_rows AS (
  SELECT id FROM before_rows
  UNION ALL
  SELECT id FROM target
  UNION ALL
  SELECT id FROM after_rows
)
SELECT
  target.id AS target_id,
  target.turn_message_seq AS target_turn_message_seq,
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
FROM window_rows w
CROSS JOIN target
JOIN bot_history_messages m ON m.id = w.id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesAfterBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND julianday(m.created_at) > julianday(sqlc.arg(created_at))
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesAfterMessageBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND (m.turn_position, m.turn_message_seq, julianday(m.created_at), m.id)
    > (
      SELECT c.turn_position, c.turn_message_seq, julianday(c.created_at), c.id
      FROM bot_history_messages c
      WHERE c.session_id = sqlc.arg(session_id)
        AND c.turn_visible = 1
        AND c.turn_id IS NOT NULL
        AND c.turn_position IS NOT NULL
        AND c.turn_message_seq IS NOT NULL
        AND c.id = sqlc.arg(after_message_id)
      LIMIT 1
  )
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeCursorBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND (
    m.turn_position < sqlc.arg(cursor_turn_position)
    OR (m.turn_position = sqlc.arg(cursor_turn_position) AND m.turn_message_seq < sqlc.arg(cursor_turn_message_seq))
    OR (m.turn_position = sqlc.arg(cursor_turn_position) AND m.turn_message_seq = sqlc.arg(cursor_turn_message_seq) AND julianday(m.created_at) < julianday(sqlc.arg(cursor_created_at)))
    OR (m.turn_position = sqlc.arg(cursor_turn_position) AND m.turn_message_seq = sqlc.arg(cursor_turn_message_seq) AND julianday(m.created_at) = julianday(sqlc.arg(cursor_created_at)) AND m.id < sqlc.arg(cursor_message_id))
  )
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesAfterCursorBySession :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND (
    m.turn_position > sqlc.arg(cursor_turn_position)
    OR (m.turn_position = sqlc.arg(cursor_turn_position) AND m.turn_message_seq > sqlc.arg(cursor_turn_message_seq))
    OR (m.turn_position = sqlc.arg(cursor_turn_position) AND m.turn_message_seq = sqlc.arg(cursor_turn_message_seq) AND julianday(m.created_at) > julianday(sqlc.arg(cursor_created_at)))
    OR (m.turn_position = sqlc.arg(cursor_turn_position) AND m.turn_message_seq = sqlc.arg(cursor_turn_message_seq) AND julianday(m.created_at) = julianday(sqlc.arg(cursor_created_at)) AND m.id > sqlc.arg(cursor_message_id))
  )
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListVisibleMessagesFromBySession :many
WITH cursor_message AS (
  SELECT m.turn_position
  FROM bot_history_messages m
  WHERE m.session_id = sqlc.arg(session_id)
    AND m.turn_visible = 1
    AND m.turn_id IS NOT NULL
    AND m.turn_position IS NOT NULL
    AND m.id = sqlc.arg(message_id)
  LIMIT 1
)
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
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
CROSS JOIN cursor_message cursor
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL
  AND m.turn_position >= cursor.turn_position
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
WHERE id IN (sqlc.slice(ids));

-- name: ListObservedConversationsByChannelIdentity :many
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at) AS last_observed_at
  FROM bot_visible_history_messages m
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
  FROM bot_visible_history_messages m
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
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(session_id) IS NULL OR m.session_id = sqlc.narg(session_id))
  AND (sqlc.narg(contact_id) IS NULL OR m.sender_channel_identity_id = sqlc.narg(contact_id))
  AND (sqlc.narg(start_time) IS NULL OR julianday(m.created_at) >= julianday(sqlc.narg(start_time)))
  AND (sqlc.narg(end_time) IS NULL OR julianday(m.created_at) <= julianday(sqlc.narg(end_time)))
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
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: MarkMessagesCompacted :exec
UPDATE bot_history_messages
SET compact_id = sqlc.arg(compact_id)
WHERE id IN (sqlc.slice(ids));

-- name: ListUncompactedMessagesBySession :many
SELECT id, bot_id, session_id, role, content, usage, sender_channel_identity_id, compact_id, created_at
FROM bot_visible_history_messages
WHERE session_id = sqlc.arg(session_id)
  AND compact_id IS NULL
ORDER BY turn_position ASC, turn_message_seq ASC, created_at ASC, id ASC;
