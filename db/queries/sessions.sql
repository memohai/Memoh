-- name: CreateSession :one
INSERT INTO bot_sessions (
  bot_id, route_id, channel_type, title, metadata
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(route_id)::uuid,
  sqlc.narg(channel_type)::text,
  sqlc.arg(title),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetSessionByID :one
SELECT *
FROM bot_sessions
WHERE id = $1
  AND deleted_at IS NULL;

-- name: ListSessionsByBot :many
SELECT *
FROM bot_sessions
WHERE bot_id = sqlc.arg(bot_id)
  AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: ListSessionsByRoute :many
SELECT *
FROM bot_sessions
WHERE route_id = sqlc.arg(route_id)
  AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: UpdateSessionTitle :one
UPDATE bot_sessions
SET title = sqlc.arg(title), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateSessionMetadata :one
UPDATE bot_sessions
SET metadata = sqlc.arg(metadata), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteSession :exec
UPDATE bot_sessions
SET deleted_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: TouchSession :exec
UPDATE bot_sessions
SET updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetActiveSessionForRoute :one
SELECT s.*
FROM bot_sessions s
JOIN bot_channel_routes r ON r.active_session_id = s.id
WHERE r.id = sqlc.arg(route_id)
  AND s.deleted_at IS NULL;

-- name: SoftDeleteSessionsByBot :exec
UPDATE bot_sessions
SET deleted_at = now(), updated_at = now()
WHERE bot_id = sqlc.arg(bot_id) AND deleted_at IS NULL;
