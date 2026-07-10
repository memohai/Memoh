-- name: CreateCompactionLog :one
INSERT INTO bot_history_message_compacts (bot_id, session_id)
VALUES ($1, $2)
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
          artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
          superseded_by, superseded_at, started_at, completed_at;

-- name: CompleteCompactionLog :one
UPDATE bot_history_message_compacts
SET status = $2,
    summary = $3,
    message_count = $4,
    error_message = $5,
    usage = $6,
    model_id = $7,
    coverage = $8,
    anchor_start_ms = $9,
    anchor_end_ms = $10,
    completed_at = now()
WHERE id = $1
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
          artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
          superseded_by, superseded_at, started_at, completed_at;

-- name: GetCompactionLogByID :one
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
       artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
       superseded_by, superseded_at, started_at, completed_at
FROM bot_history_message_compacts
WHERE id = $1;

-- name: ListCompactionArtifactParentIDsBySuccessor :many
SELECT id
FROM bot_history_message_compacts
WHERE superseded_by = sqlc.arg(successor_id)
  AND bot_id = sqlc.arg(bot_id)
  AND session_id IS NOT DISTINCT FROM sqlc.narg(session_id)::uuid
  AND status = 'ok'
ORDER BY id ASC;

-- name: ListCompactionArtifactParentEdges :many
SELECT parent_id, ordinal
FROM bot_history_message_compact_parent_edges
WHERE artifact_id = $1
ORDER BY ordinal ASC;

-- name: ListCompactionLogsByBot :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
       artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
       superseded_by, superseded_at, started_at, completed_at
FROM bot_history_message_compacts
WHERE bot_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;

-- name: CountCompactionLogsByBot :one
SELECT count(*) FROM bot_history_message_compacts WHERE bot_id = $1;

-- name: ListCompactionArtifactLineageBySession :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
       artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
       superseded_by, superseded_at, started_at, completed_at
FROM bot_history_message_compacts c
WHERE c.session_id = $1
  AND (
    c.status = 'ok'
    OR EXISTS (
      SELECT 1
      FROM bot_history_message_compacts parent
      WHERE parent.session_id = $1
        AND parent.status = 'ok'
        AND parent.superseded_by = c.id
    )
  )
ORDER BY c.anchor_start_ms ASC, c.started_at ASC, c.id ASC;

-- name: DeleteCompactionLogsByBot :exec
WITH target_compacts AS MATERIALIZED (
  SELECT compact.id
  FROM bot_history_message_compacts AS compact
  WHERE compact.bot_id = sqlc.arg(bot_id)
), deleted_parent_edges AS (
  DELETE FROM bot_history_message_compact_parent_edges
  WHERE artifact_id IN (SELECT id FROM target_compacts)
)
DELETE FROM bot_history_message_compacts
WHERE id IN (SELECT id FROM target_compacts);
