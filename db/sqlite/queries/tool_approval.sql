-- name: CreateToolApprovalRequest :one
INSERT INTO tool_approval_requests (
  id, bot_id, session_id, route_id, channel_identity_id,
  tool_call_id, tool_name, operation, tool_input, short_id,
  requested_by_channel_identity_id, requested_message_id,
  persist_turn_id,
  source_platform, reply_target, conversation_type
) SELECT
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
  sqlc.narg(persist_turn_id),
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type)
WHERE EXISTS (
  SELECT 1
  FROM bot_sessions
  WHERE id = sqlc.arg(session_id)
    AND bot_id = sqlc.arg(bot_id)
)
ON CONFLICT (session_id, tool_call_id) WHERE persist_turn_id IS NULL DO UPDATE
SET tool_input = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.tool_input
  ELSE tool_approval_requests.tool_input
END
RETURNING *;

-- name: CreateToolApprovalRequestForTurn :one
INSERT INTO tool_approval_requests (
  id, bot_id, session_id, route_id, channel_identity_id,
  tool_call_id, tool_name, operation, tool_input, short_id,
  requested_by_channel_identity_id, requested_message_id,
  persist_turn_id,
  source_platform, reply_target, conversation_type
) SELECT
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
  sqlc.arg(persist_turn_id),
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type)
WHERE EXISTS (
  SELECT 1
  FROM bot_sessions
  WHERE id = sqlc.arg(session_id)
    AND bot_id = sqlc.arg(bot_id)
)
AND EXISTS (
  SELECT 1
  FROM bot_history_turns
  WHERE id = sqlc.arg(persist_turn_id)
    AND bot_id = sqlc.arg(bot_id)
    AND owner_session_id = sqlc.arg(session_id)
)
ON CONFLICT (session_id, tool_call_id, persist_turn_id) WHERE persist_turn_id IS NOT NULL DO UPDATE
SET tool_input = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.tool_input
  ELSE tool_approval_requests.tool_input
END
RETURNING *;

-- name: GetToolApprovalRequest :one
SELECT *
FROM tool_approval_requests
WHERE id = sqlc.arg(id);

-- name: GetPendingToolApprovalByVisibleRequestID :one
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.id = sqlc.arg(id)
  AND tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND tar.status = 'pending'
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns));

-- name: GetPendingToolApprovalByBaseHeadRequestID :one
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.id = sqlc.arg(id)
  AND tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND tar.status = 'pending'
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns));

-- name: GetPendingToolApprovalBySessionShortID :one
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND tar.short_id = sqlc.arg(short_id)
  AND tar.status = 'pending'
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns));

-- name: GetLatestPendingToolApprovalBySession :one
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND tar.status = 'pending'
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY tar.created_at DESC, tar.short_id DESC
LIMIT 1;

-- name: GetPendingToolApprovalByReplyMessage :one
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND tar.prompt_external_message_id = sqlc.arg(prompt_external_message_id)
  AND tar.status = 'pending'
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY tar.created_at DESC
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
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND tar.status = 'pending'
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY tar.created_at ASC, tar.short_id ASC;

-- name: ListToolApprovalsBySession :many
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY tar.created_at ASC, tar.short_id ASC;

-- name: ListToolApprovalsBySessionTurnGraph :many
WITH RECURSIVE visible_turns(id, parent_turn_id) AS (
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT DISTINCT visible_turns.id FROM visible_turns))
ORDER BY tar.created_at ASC, tar.short_id ASC;
