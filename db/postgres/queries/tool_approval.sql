-- name: CreateToolApprovalRequest :one
WITH locked_session AS (
  SELECT id, bot_id
  FROM bot_sessions
  WHERE id = sqlc.arg(session_id)
    AND bot_id = sqlc.arg(bot_id)
  FOR UPDATE
),
next_short_id AS (
  SELECT COALESCE(MAX(tool_approval_requests.short_id), 0) + 1 AS short_id
  FROM locked_session
  LEFT JOIN tool_approval_requests ON tool_approval_requests.session_id = locked_session.id
)
INSERT INTO tool_approval_requests (
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  operation,
  tool_input,
  short_id,
  requested_by_channel_identity_id,
  requested_message_id,
  persist_turn_id,
  source_platform,
  reply_target,
  conversation_type
) SELECT
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_identity_id),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  sqlc.arg(operation),
  sqlc.arg(tool_input),
  next_short_id.short_id,
  sqlc.narg(requested_by_channel_identity_id),
  sqlc.narg(requested_message_id),
  sqlc.narg(persist_turn_id),
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type)
FROM locked_session
CROSS JOIN next_short_id
ON CONFLICT (session_id, tool_call_id) WHERE persist_turn_id IS NULL DO UPDATE
SET tool_input = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.tool_input
  ELSE tool_approval_requests.tool_input
END
RETURNING *;

-- name: CreateToolApprovalRequestForTurn :one
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
  SELECT COALESCE(MAX(tool_approval_requests.short_id), 0) + 1 AS short_id
  FROM locked_session
  LEFT JOIN tool_approval_requests ON tool_approval_requests.session_id = locked_session.id
)
INSERT INTO tool_approval_requests (
  bot_id,
  session_id,
  route_id,
  channel_identity_id,
  tool_call_id,
  tool_name,
  operation,
  tool_input,
  short_id,
  requested_by_channel_identity_id,
  requested_message_id,
  persist_turn_id,
  source_platform,
  reply_target,
  conversation_type
) SELECT
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_identity_id),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  sqlc.arg(operation),
  sqlc.arg(tool_input),
  next_short_id.short_id,
  sqlc.narg(requested_by_channel_identity_id),
  sqlc.narg(requested_message_id),
  persist_turn.id,
  sqlc.arg(source_platform),
  sqlc.arg(reply_target),
  sqlc.arg(conversation_type)
FROM locked_session
CROSS JOIN next_short_id
CROSS JOIN persist_turn
ON CONFLICT (session_id, tool_call_id, persist_turn_id) WHERE persist_turn_id IS NOT NULL DO UPDATE
SET tool_input = CASE
  WHEN tool_approval_requests.status = 'pending' THEN EXCLUDED.tool_input
  ELSE tool_approval_requests.tool_input
END
RETURNING *;

-- name: GetToolApprovalRequest :one
SELECT *
FROM tool_approval_requests
WHERE id = $1;

-- name: GetPendingToolApprovalByVisibleRequestID :one
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

-- name: CancelPendingToolApprovalsBySession :many
UPDATE tool_approval_requests
SET status = 'cancelled',
    decision_reason = sqlc.arg(reason),
    decided_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND status = 'pending'
RETURNING *;

-- name: ListPendingToolApprovalsBySession :many
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns))
ORDER BY tar.created_at ASC, tar.short_id ASC;

-- name: ListToolApprovalsBySessionToolCalls :many
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND tar.tool_call_id = ANY(sqlc.arg(tool_call_ids)::text[])
  AND (
    tar.persist_turn_id IS NULL
    OR tar.persist_turn_id = ANY(sqlc.arg(turn_ids)::uuid[])
  )
ORDER BY tar.created_at ASC, tar.short_id ASC;

-- name: ListToolApprovalsBySessionTurnGraph :many
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
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = sqlc.arg(bot_id)
  AND tar.session_id = sqlc.arg(session_id)
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT DISTINCT visible_turns.id FROM visible_turns))
ORDER BY tar.created_at ASC, tar.short_id ASC;
