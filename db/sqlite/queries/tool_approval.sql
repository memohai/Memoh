-- name: CreateToolApprovalRequest :one
INSERT INTO tool_approval_requests (
  id, bot_id, session_id, route_id, channel_identity_id,
  tool_call_id, tool_name, operation, tool_input, short_id,
  requested_by_channel_identity_id, requested_message_id,
  persist_branch_id, persist_turn_id,
  source_platform, reply_target, conversation_type
) VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_identity_id),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  sqlc.arg(operation),
  sqlc.arg(tool_input),
  (
    SELECT COALESCE(MAX(short_id), 0) + 1
    FROM tool_approval_requests
    WHERE session_id = sqlc.arg(session_id)
  ),
  sqlc.narg(requested_by_channel_identity_id),
  sqlc.narg(requested_message_id),
  sqlc.narg(persist_branch_id),
  sqlc.narg(persist_turn_id),
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type)
)
ON CONFLICT (session_id, tool_call_id) DO UPDATE
SET tool_input = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.tool_input
  ELSE tool_approval_requests.tool_input
END,
requested_by_channel_identity_id = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.requested_by_channel_identity_id
  ELSE tool_approval_requests.requested_by_channel_identity_id
END,
requested_message_id = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.requested_message_id
  ELSE tool_approval_requests.requested_message_id
END,
persist_branch_id = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.persist_branch_id
  ELSE tool_approval_requests.persist_branch_id
END,
persist_turn_id = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.persist_turn_id
  ELSE tool_approval_requests.persist_turn_id
END,
source_platform = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.source_platform
  ELSE tool_approval_requests.source_platform
END,
reply_target = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.reply_target
  ELSE tool_approval_requests.reply_target
END,
conversation_type = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.conversation_type
  ELSE tool_approval_requests.conversation_type
END
RETURNING *;

-- name: GetToolApprovalRequest :one
SELECT *
FROM tool_approval_requests
WHERE id = sqlc.arg(id);

-- name: GetPendingToolApprovalBySessionShortID :one
SELECT *
FROM tool_approval_requests
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND short_id = sqlc.arg(short_id)
  AND status = 'pending';

-- name: GetLatestPendingToolApprovalBySession :one
SELECT *
FROM tool_approval_requests
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND status = 'pending'
ORDER BY created_at DESC, short_id DESC
LIMIT 1;

-- name: GetPendingToolApprovalByReplyMessage :one
SELECT *
FROM tool_approval_requests
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND prompt_external_message_id = sqlc.arg(prompt_external_message_id)
  AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateToolApprovalPromptMessage :one
UPDATE tool_approval_requests
SET prompt_message_id = sqlc.narg(prompt_message_id),
    prompt_external_message_id = sqlc.arg(prompt_external_message_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateToolApprovalPersistContext :one
UPDATE tool_approval_requests
SET persist_branch_id = sqlc.narg(persist_branch_id),
    persist_turn_id = sqlc.narg(persist_turn_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ApproveToolApprovalRequest :one
UPDATE tool_approval_requests
SET status = 'approved',
    decision_reason = sqlc.arg(reason),
    decided_by_channel_identity_id = sqlc.narg(decided_by_channel_identity_id),
    decided_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND status = 'pending'
RETURNING *;

-- name: RejectToolApprovalRequest :one
UPDATE tool_approval_requests
SET status = 'rejected',
    decision_reason = sqlc.arg(reason),
    decided_by_channel_identity_id = sqlc.narg(decided_by_channel_identity_id),
    decided_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND status = 'pending'
RETURNING *;

-- name: CancelPendingToolApprovalsBySession :many
UPDATE tool_approval_requests
SET status = 'cancelled',
    decision_reason = sqlc.arg(reason),
    decided_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND status = 'pending'
RETURNING *;

-- name: ListPendingToolApprovalsBySession :many
SELECT *
FROM tool_approval_requests
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND status = 'pending'
ORDER BY created_at ASC, short_id ASC;

-- name: ListToolApprovalsBySession :many
SELECT *
FROM tool_approval_requests
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
ORDER BY created_at ASC, short_id ASC;
