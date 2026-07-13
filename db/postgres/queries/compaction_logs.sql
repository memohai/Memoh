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
SELECT c.id, c.bot_id, c.session_id, c.status, c.summary, c.message_count, c.error_message, c.usage, c.model_id,
       c.artifact_version, c.coverage, c.anchor_start_ms, c.anchor_end_ms, c.artifact_level, c.parent_ids,
       c.superseded_by, c.superseded_at, c.started_at, c.completed_at
FROM bot_history_message_compacts c
JOIN bot_sessions owner_session
  ON owner_session.id = $1
 AND owner_session.bot_id = c.bot_id
WHERE c.session_id = owner_session.id
  AND (
    c.status = 'ok'
    OR EXISTS (
      SELECT 1
      FROM bot_history_message_compacts parent
      WHERE parent.bot_id = owner_session.bot_id
        AND parent.session_id = owner_session.id
        AND parent.status = 'ok'
        AND parent.superseded_by = c.id
    )
  )
ORDER BY c.anchor_start_ms ASC, c.started_at ASC, c.id ASC;

-- name: DeleteCompactionLogsByBot :exec
DELETE FROM bot_history_message_compacts WHERE bot_id = $1;
