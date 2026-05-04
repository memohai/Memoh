-- name: CreateToolApprovalRequest :one
INSERT INTO tool_approval_requests (
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  tool_input,
  short_id,
  requested_by_channel_identity_id,
  requested_message_id,
  source_platform,
  reply_target,
  conversation_type
) VALUES (
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_identity_id),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  sqlc.arg(tool_input),
  (
    SELECT COALESCE(MAX(short_id), 0) + 1
    FROM tool_approval_requests
    WHERE session_id = sqlc.arg(session_id)
  ),
  sqlc.narg(requested_by_channel_identity_id),
  sqlc.narg(requested_message_id),
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type)
)
ON CONFLICT (session_id, tool_call_id) DO UPDATE
SET tool_input = EXCLUDED.tool_input
RETURNING *;

-- name: GetToolApprovalRequest :one
SELECT *
FROM tool_approval_requests
WHERE id = $1;

-- name: GetPendingToolApprovalBySessionShortID :one
SELECT *
FROM tool_approval_requests
WHERE bot_id = $1
  AND session_id = $2
  AND short_id = $3
  AND status = 'pending';

-- name: GetLatestPendingToolApprovalBySession :one
SELECT *
FROM tool_approval_requests
WHERE bot_id = $1
  AND session_id = $2
  AND status = 'pending'
ORDER BY created_at DESC, short_id DESC
LIMIT 1;

-- name: GetPendingToolApprovalByReplyMessage :one
SELECT *
FROM tool_approval_requests
WHERE bot_id = $1
  AND session_id = $2
  AND prompt_external_message_id = $3
  AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateToolApprovalPromptMessage :one
UPDATE tool_approval_requests
SET prompt_message_id = sqlc.narg(prompt_message_id),
    prompt_external_message_id = sqlc.arg(prompt_external_message_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ApproveToolApprovalRequest :one
UPDATE tool_approval_requests
SET status = 'approved',
    decision_reason = sqlc.arg(reason),
    decided_by_channel_identity_id = sqlc.narg(decided_by_channel_identity_id),
    decided_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'pending'
RETURNING *;

-- name: RejectToolApprovalRequest :one
UPDATE tool_approval_requests
SET status = 'rejected',
    decision_reason = sqlc.arg(reason),
    decided_by_channel_identity_id = sqlc.narg(decided_by_channel_identity_id),
    decided_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'pending'
RETURNING *;

-- name: ListPendingToolApprovalsBySession :many
SELECT *
FROM tool_approval_requests
WHERE bot_id = $1
  AND session_id = $2
  AND status = 'pending'
ORDER BY created_at ASC, short_id ASC;

-- name: ListToolApprovalsBySession :many
SELECT *
FROM tool_approval_requests
WHERE bot_id = $1
  AND session_id = $2
ORDER BY created_at ASC, short_id ASC;
