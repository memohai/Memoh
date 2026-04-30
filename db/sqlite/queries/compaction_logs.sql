-- name: CreateCompactionLog :one
INSERT INTO bot_history_message_compacts (id, bot_id, session_id)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(session_id)
)
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at;

-- name: CompleteCompactionLog :one
UPDATE bot_history_message_compacts
SET status = sqlc.arg(status),
    summary = sqlc.arg(summary),
    message_count = sqlc.arg(message_count),
    error_message = sqlc.arg(error_message),
    usage = sqlc.arg(usage),
    model_id = sqlc.arg(model_id),
    completed_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at;

-- name: GetCompactionLogByID :one
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at
FROM bot_history_message_compacts
WHERE id = sqlc.arg(id);

-- name: ListCompactionLogsByBot :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at
FROM bot_history_message_compacts
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY started_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountCompactionLogsByBot :one
SELECT count(*) FROM bot_history_message_compacts WHERE bot_id = sqlc.arg(bot_id);

-- name: ListCompactionLogsBySession :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id, started_at, completed_at
FROM bot_history_message_compacts
WHERE session_id = sqlc.arg(session_id)
ORDER BY started_at ASC;

-- name: DeleteCompactionLogsByBot :exec
DELETE FROM bot_history_message_compacts WHERE bot_id = sqlc.arg(bot_id);
