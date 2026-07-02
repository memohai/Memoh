-- name: CreateUserInputRequest :one
WITH locked_session AS (
  SELECT id, bot_id
  FROM bot_sessions
  WHERE id = sqlc.arg(session_id)
    AND bot_id = sqlc.arg(bot_id)
  FOR UPDATE
),
next_short_id AS (
  SELECT COALESCE(MAX(user_input_requests.short_id), 0) + 1 AS short_id
  FROM locked_session
  LEFT JOIN user_input_requests ON user_input_requests.session_id = locked_session.id
)
INSERT INTO user_input_requests (
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  short_id,
  input_json,
  ui_payload_json,
  provider_metadata,
  requested_by_channel_identity_id,
  persist_turn_id,
  source_platform,
  reply_target,
  conversation_type,
  expires_at
) SELECT
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_identity_id),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  next_short_id.short_id,
  sqlc.arg(input_json),
  sqlc.arg(ui_payload_json),
  sqlc.arg(provider_metadata),
  sqlc.narg(requested_by_channel_identity_id),
  sqlc.narg(persist_turn_id),
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type),
  sqlc.narg(expires_at)
FROM locked_session
CROSS JOIN next_short_id
ON CONFLICT (session_id, tool_call_id) WHERE persist_turn_id IS NULL DO UPDATE
SET input_json = EXCLUDED.input_json,
    ui_payload_json = EXCLUDED.ui_payload_json,
    provider_metadata = EXCLUDED.provider_metadata,
    requested_by_channel_identity_id = EXCLUDED.requested_by_channel_identity_id,
    source_platform = EXCLUDED.source_platform,
    reply_target = EXCLUDED.reply_target,
    conversation_type = EXCLUDED.conversation_type,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
WHERE user_input_requests.status = 'pending'
  AND (user_input_requests.expires_at IS NULL OR user_input_requests.expires_at > now())
RETURNING *;

-- name: CreateUserInputRequestForTurn :one
WITH locked_session AS (
  SELECT id, bot_id
  FROM bot_sessions
  WHERE id = sqlc.arg(session_id)
    AND bot_id = sqlc.arg(bot_id)
  FOR UPDATE
),
persist_turn AS (
  SELECT t.id
  FROM locked_session
  JOIN bot_history_turns t
    ON t.id = sqlc.arg(persist_turn_id)
   AND t.bot_id = locked_session.bot_id
   AND t.owner_session_id = locked_session.id
),
next_short_id AS (
  SELECT COALESCE(MAX(user_input_requests.short_id), 0) + 1 AS short_id
  FROM locked_session
  LEFT JOIN user_input_requests ON user_input_requests.session_id = locked_session.id
)
INSERT INTO user_input_requests (
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  short_id,
  input_json,
  ui_payload_json,
  provider_metadata,
  requested_by_channel_identity_id,
  persist_turn_id,
  source_platform,
  reply_target,
  conversation_type,
  expires_at
) SELECT
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_identity_id),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  next_short_id.short_id,
  sqlc.arg(input_json),
  sqlc.arg(ui_payload_json),
  sqlc.arg(provider_metadata),
  sqlc.narg(requested_by_channel_identity_id),
  persist_turn.id,
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type),
  sqlc.narg(expires_at)
FROM locked_session
CROSS JOIN next_short_id
CROSS JOIN persist_turn
ON CONFLICT (session_id, tool_call_id, persist_turn_id) WHERE persist_turn_id IS NOT NULL DO UPDATE
SET input_json = EXCLUDED.input_json,
    ui_payload_json = EXCLUDED.ui_payload_json,
    provider_metadata = EXCLUDED.provider_metadata,
    requested_by_channel_identity_id = EXCLUDED.requested_by_channel_identity_id,
    source_platform = EXCLUDED.source_platform,
    reply_target = EXCLUDED.reply_target,
    conversation_type = EXCLUDED.conversation_type,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
WHERE user_input_requests.status = 'pending'
  AND (user_input_requests.expires_at IS NULL OR user_input_requests.expires_at > now())
RETURNING *;

-- name: GetUserInputRequest :one
SELECT *
FROM user_input_requests
WHERE id = $1;

-- name: GetPendingUserInputByVisibleRequestID :one
WITH RECURSIVE visible_turns AS (
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
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.id = sqlc.arg(id)
  AND uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND uir.status = 'pending'
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns));

-- name: GetPendingUserInputByBaseHeadRequestID :one
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
    AND h.head_turn_id = sqlc.arg(base_head_turn_id)
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = sqlc.arg(session_id)
    AND s.deleted_at IS NULL
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.id = sqlc.arg(id)
  AND uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND uir.status = 'pending'
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns));

-- name: GetUserInputRequestBySessionToolCall :one
SELECT *
FROM user_input_requests
WHERE session_id = $1
  AND tool_call_id = $2
  AND persist_turn_id IS NULL;

-- name: GetUserInputRequestBySessionToolCallTurn :one
SELECT *
FROM user_input_requests
WHERE session_id = $1
  AND tool_call_id = $2
  AND persist_turn_id = $3;

-- name: GetPendingUserInputBySessionShortID :one
WITH RECURSIVE visible_turns AS (
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
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND uir.short_id = sqlc.arg(short_id)
  AND uir.status = 'pending'
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns));

-- name: GetLatestPendingUserInputBySession :one
WITH RECURSIVE visible_turns AS (
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
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND uir.status = 'pending'
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY uir.created_at DESC, uir.short_id DESC
LIMIT 1;

-- name: GetPendingUserInputByReplyMessage :one
WITH RECURSIVE visible_turns AS (
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
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND uir.prompt_external_message_id = sqlc.arg(prompt_external_message_id)
  AND uir.status = 'pending'
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY uir.created_at DESC
LIMIT 1;

-- name: UpdateUserInputPromptMessage :one
UPDATE user_input_requests
SET prompt_message_id = sqlc.narg(prompt_message_id),
    prompt_external_message_id = sqlc.arg(prompt_external_message_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUserInputAssistantMessage :one
UPDATE user_input_requests
SET assistant_message_id = sqlc.narg(assistant_message_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUserInputToolResultMessage :one
UPDATE user_input_requests
SET tool_result_message_id = sqlc.narg(tool_result_message_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SubmitUserInputRequest :one
UPDATE user_input_requests
SET status = 'submitted',
    result_json = sqlc.arg(result_json),
    responded_by_channel_identity_id = sqlc.narg(responded_by_channel_identity_id),
    responded_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'pending'
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: CancelUserInputRequest :one
UPDATE user_input_requests
SET status = 'canceled',
    result_json = sqlc.arg(result_json),
    responded_by_channel_identity_id = sqlc.narg(responded_by_channel_identity_id),
    responded_at = now(),
    canceled_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'pending'
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: CancelPendingUserInputsBySession :many
UPDATE user_input_requests
SET status = 'canceled',
    result_json = sqlc.arg(result_json),
    responded_at = now(),
    canceled_at = now(),
    updated_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND status = 'pending'
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: FailUserInputRequest :one
UPDATE user_input_requests
SET status = 'failed',
    result_json = sqlc.arg(result_json),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'pending'
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: ListPendingUserInputsBySession :many
WITH RECURSIVE visible_turns AS (
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
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND uir.status = 'pending'
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY uir.created_at ASC, uir.short_id ASC;

-- name: ListUserInputsBySession :many
WITH RECURSIVE visible_turns AS (
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
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY uir.created_at ASC, uir.short_id ASC;

-- name: ListUserInputsBySessionToolCalls :many
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND uir.tool_call_id = ANY(sqlc.arg(tool_call_ids)::text[])
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (
    uir.persist_turn_id IS NULL
    OR uir.persist_turn_id = ANY(sqlc.arg(turn_ids)::uuid[])
  )
ORDER BY uir.created_at ASC, uir.short_id ASC;

-- name: ListUserInputsBySessionTurnGraph :many
WITH RECURSIVE visible_turns AS (
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
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = sqlc.arg(bot_id)
  AND uir.session_id = sqlc.arg(session_id)
  AND (uir.persist_turn_id IS NULL OR uir.persist_turn_id IN (SELECT DISTINCT visible_turns.id FROM visible_turns))
ORDER BY uir.created_at ASC, uir.short_id ASC;
