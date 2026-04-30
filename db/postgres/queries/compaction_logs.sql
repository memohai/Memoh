-- name: CreateCompactionLog :one
INSERT INTO bot_history_message_compacts (bot_id, session_id)
VALUES ($1, $2)
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at;

-- name: CompleteCompactionLog :one
UPDATE bot_history_message_compacts
SET status = $2,
    summary = $3,
    message_count = $4,
    error_message = $5,
    usage = $6,
    model_id = $7,
    completed_at = now()
WHERE id = $1
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at;

-- name: GetCompactionLogByID :one
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at
FROM bot_history_message_compacts
WHERE id = $1;

-- name: ListCompactionLogsByBot :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at
FROM bot_history_message_compacts
WHERE bot_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;

-- name: CountCompactionLogsByBot :one
SELECT count(*) FROM bot_history_message_compacts WHERE bot_id = $1;

-- name: ListCompactionLogsBySession :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at
FROM bot_history_message_compacts
WHERE session_id = $1
ORDER BY started_at ASC;

-- name: DeleteCompactionLogsByBot :exec
DELETE FROM bot_history_message_compacts WHERE bot_id = $1;
