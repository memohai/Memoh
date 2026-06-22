-- name: CreateSession :one
INSERT INTO bot_sessions (
  bot_id, route_id, channel_type, type, title, metadata, parent_session_id, created_by_user_id
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(route_id)::uuid,
  sqlc.narg(channel_type)::text,
  sqlc.arg(type),
  sqlc.arg(title),
  sqlc.arg(metadata),
  sqlc.narg(parent_session_id)::uuid,
  sqlc.narg(created_by_user_id)::uuid
)
RETURNING *;

-- name: GetSessionByID :one
SELECT *
FROM bot_sessions
WHERE id = $1
  AND deleted_at IS NULL;

-- name: ListSessionsByBot :many
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.title, s.metadata,
  s.parent_session_id, s.created_by_user_id, s.created_at, s.updated_at, s.deleted_at,
  r.metadata AS route_metadata,
  r.conversation_type AS route_conversation_type
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.deleted_at IS NULL
ORDER BY s.updated_at DESC;

-- name: ListSessionsByBotAndCreatedByUser :many
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.title, s.metadata,
  s.parent_session_id, s.created_by_user_id, s.created_at, s.updated_at, s.deleted_at,
  r.metadata AS route_metadata,
  r.conversation_type AS route_conversation_type
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.created_by_user_id = sqlc.arg(created_by_user_id)
  AND s.deleted_at IS NULL
ORDER BY s.updated_at DESC;

-- name: ListSessionsByBotPaged :many
-- Cursor uses (updated_at, id) so pages stay stable when many rows share an
-- updated_at. Callers always pass an explicit types filter; to opt out of
-- filtering, pass every known type.
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.title, s.metadata,
  s.parent_session_id, s.created_by_user_id, s.created_at, s.updated_at, s.deleted_at,
  r.metadata AS route_metadata,
  r.conversation_type AS route_conversation_type
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.deleted_at IS NULL
  AND s.type = ANY(sqlc.arg(types)::text[])
  AND (
    NOT sqlc.arg(use_cursor)::bool
    OR (s.updated_at, s.id) < (sqlc.arg(cursor_updated_at)::timestamptz, sqlc.arg(cursor_id)::uuid)
  )
ORDER BY s.updated_at DESC, s.id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: ListSessionsByBotAndCreatedByUserPaged :many
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.title, s.metadata,
  s.parent_session_id, s.created_by_user_id, s.created_at, s.updated_at, s.deleted_at,
  r.metadata AS route_metadata,
  r.conversation_type AS route_conversation_type
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.created_by_user_id = sqlc.arg(created_by_user_id)
  AND s.deleted_at IS NULL
  AND s.type = ANY(sqlc.arg(types)::text[])
  AND (
    NOT sqlc.arg(use_cursor)::bool
    OR (s.updated_at, s.id) < (sqlc.arg(cursor_updated_at)::timestamptz, sqlc.arg(cursor_id)::uuid)
  )
ORDER BY s.updated_at DESC, s.id DESC
LIMIT sqlc.arg(limit_count)::int;

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

-- name: UpdateSessionTypeAndMetadata :one
UPDATE bot_sessions
SET type = sqlc.arg(type), metadata = sqlc.arg(metadata), updated_at = now()
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

-- name: ListSubagentSessionsByParent :many
SELECT *
FROM bot_sessions
WHERE parent_session_id = sqlc.arg(parent_session_id)
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: SoftDeleteSessionsByBot :exec
UPDATE bot_sessions
SET deleted_at = now(), updated_at = now()
WHERE bot_id = sqlc.arg(bot_id) AND deleted_at IS NULL;
