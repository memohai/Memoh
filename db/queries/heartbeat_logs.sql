-- name: CreateHeartbeatLog :one
INSERT INTO bot_heartbeat_logs (bot_id, started_at)
VALUES ($1, now())
RETURNING id, bot_id, status, result_text, error_message, usage, started_at, completed_at;

-- name: CompleteHeartbeatLog :one
UPDATE bot_heartbeat_logs
SET status = $2,
    result_text = $3,
    error_message = $4,
    usage = $5,
    completed_at = now()
WHERE id = $1
RETURNING id, bot_id, status, result_text, error_message, usage, started_at, completed_at;

-- name: ListHeartbeatLogsByBot :many
SELECT id, bot_id, status, result_text, error_message, usage, started_at, completed_at
FROM bot_heartbeat_logs
WHERE bot_id = $1
  AND ($2::timestamptz IS NULL OR started_at < $2::timestamptz)
ORDER BY started_at DESC
LIMIT $3;

-- name: DeleteHeartbeatLogsByBot :exec
DELETE FROM bot_heartbeat_logs WHERE bot_id = $1;
