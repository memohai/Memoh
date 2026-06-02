-- name: CreateUserInputRequest :one
INSERT INTO user_input_requests (
  id,
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
  source_platform,
  reply_target,
  conversation_type,
  expires_at
) VALUES (
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || '4' || substr(lower(hex(randomblob(2))), 2) || '-' || substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_identity_id),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  (
    SELECT COALESCE(MAX(short_id), 0) + 1
    FROM user_input_requests
    WHERE session_id = sqlc.arg(session_id)
  ),
  sqlc.arg(input_json),
  sqlc.arg(ui_payload_json),
  sqlc.arg(provider_metadata),
  sqlc.narg(requested_by_channel_identity_id),
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type),
  sqlc.narg(expires_at)
)
RETURNING *;

-- name: GetUserInputRequest :one
SELECT *
FROM user_input_requests
WHERE id = ?;

-- name: GetPendingUserInputBySessionShortID :one
SELECT *
FROM user_input_requests
WHERE bot_id = ?
  AND session_id = ?
  AND short_id = ?
  AND status = 'pending';

-- name: GetLatestPendingUserInputBySession :one
SELECT *
FROM user_input_requests
WHERE bot_id = ?
  AND session_id = ?
  AND status = 'pending'
ORDER BY created_at DESC, short_id DESC
LIMIT 1;

-- name: GetPendingUserInputByReplyMessage :one
SELECT *
FROM user_input_requests
WHERE bot_id = ?
  AND session_id = ?
  AND prompt_external_message_id = ?
  AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateUserInputPromptMessage :one
UPDATE user_input_requests
SET prompt_message_id = sqlc.narg(prompt_message_id),
    prompt_external_message_id = sqlc.arg(prompt_external_message_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUserInputAssistantMessage :one
UPDATE user_input_requests
SET assistant_message_id = sqlc.narg(assistant_message_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUserInputToolResultMessage :one
UPDATE user_input_requests
SET tool_result_message_id = sqlc.narg(tool_result_message_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SubmitUserInputRequest :one
UPDATE user_input_requests
SET status = 'submitted',
    result_json = sqlc.arg(result_json),
    responded_by_channel_identity_id = sqlc.narg(responded_by_channel_identity_id),
    responded_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND status = 'pending'
RETURNING *;

-- name: CancelUserInputRequest :one
UPDATE user_input_requests
SET status = 'canceled',
    result_json = sqlc.arg(result_json),
    responded_by_channel_identity_id = sqlc.narg(responded_by_channel_identity_id),
    responded_at = CURRENT_TIMESTAMP,
    canceled_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND status = 'pending'
RETURNING *;

-- name: FailUserInputRequest :one
UPDATE user_input_requests
SET status = 'failed',
    result_json = sqlc.arg(result_json),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND status = 'pending'
RETURNING *;

-- name: ListPendingUserInputsBySession :many
SELECT *
FROM user_input_requests
WHERE bot_id = ?
  AND session_id = ?
  AND status = 'pending'
ORDER BY created_at ASC, short_id ASC;

-- name: ListUserInputsBySession :many
SELECT *
FROM user_input_requests
WHERE bot_id = ?
  AND session_id = ?
ORDER BY created_at ASC, short_id ASC;
