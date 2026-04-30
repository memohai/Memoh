-- name: CreateHeartbeatLog :one
INSERT INTO bot_heartbeat_logs (id, bot_id, session_id, started_at)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.narg(session_id),
  CURRENT_TIMESTAMP
)
RETURNING id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at;

-- name: CompleteHeartbeatLog :one
UPDATE bot_heartbeat_logs
SET status = sqlc.arg(status),
    result_text = sqlc.arg(result_text),
    error_message = sqlc.arg(error_message),
    usage = sqlc.arg(usage),
    model_id = sqlc.arg(model_id),
    completed_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, bot_id, session_id, status, result_text, error_message, usage, model_id, started_at, completed_at;

-- name: ListHeartbeatLogsByBot :many
SELECT id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at
FROM bot_heartbeat_logs
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY started_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountHeartbeatLogsByBot :one
SELECT count(*) FROM bot_heartbeat_logs WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteHeartbeatLogsByBot :exec
DELETE FROM bot_heartbeat_logs WHERE bot_id = sqlc.arg(bot_id);
