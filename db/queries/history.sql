-- name: CreateHistory :one
INSERT INTO history (bot_id, session_id, messages, metadata, skills, timestamp)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, bot_id, session_id, messages, metadata, skills, timestamp;

-- name: ListHistoryByBotSessionSince :many
SELECT id, bot_id, session_id, messages, metadata, skills, timestamp
FROM history
WHERE bot_id = $1 AND session_id = $2 AND timestamp >= $3
ORDER BY timestamp ASC;

-- name: GetHistoryByID :one
SELECT id, bot_id, session_id, messages, metadata, skills, timestamp
FROM history
WHERE id = $1;

-- name: ListHistoryByBotSession :many
SELECT id, bot_id, session_id, messages, metadata, skills, timestamp
FROM history
WHERE bot_id = $1 AND session_id = $2
ORDER BY timestamp DESC
LIMIT $3;

-- name: DeleteHistoryByID :exec
DELETE FROM history
WHERE id = $1;

-- name: DeleteHistoryByBotSession :exec
DELETE FROM history
WHERE bot_id = $1 AND session_id = $2;

