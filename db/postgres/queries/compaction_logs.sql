-- name: CreateCompactionLog :one
INSERT INTO bot_history_message_compacts (team_id, bot_id, session_id)
VALUES (sqlc.arg(team_id)::uuid, sqlc.arg(bot_id)::uuid, sqlc.arg(session_id)::uuid)
RETURNING *;

-- name: CompleteCompactionLog :one
UPDATE bot_history_message_compacts
SET status = sqlc.arg(status),
    summary = sqlc.arg(summary),
    message_count = sqlc.arg(message_count),
    error_message = sqlc.arg(error_message),
    usage = sqlc.arg(usage),
    model_id = sqlc.narg(model_id)::uuid,
    completed_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid
RETURNING *;

-- name: GetCompactionLogByID :one
SELECT *
FROM bot_history_message_compacts
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: ListCompactionLogsByBot :many
SELECT *
FROM bot_history_message_compacts
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid
ORDER BY started_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCompactionLogsByBot :one
SELECT count(*) FROM bot_history_message_compacts
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: ListCompactionLogsBySession :many
SELECT *
FROM bot_history_message_compacts
WHERE session_id = sqlc.arg(session_id)
  AND team_id = sqlc.arg(team_id)::uuid
ORDER BY started_at ASC;

-- name: DeleteCompactionLogsByBot :exec
DELETE FROM bot_history_message_compacts
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid;
