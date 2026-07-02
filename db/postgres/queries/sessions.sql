-- name: CreateSession :one
INSERT INTO bot_sessions (
  bot_id, route_id, channel_type, type, session_mode, runtime_type, runtime_metadata, title, metadata, parent_session_id, created_by_user_id
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(route_id)::uuid,
  sqlc.narg(channel_type)::text,
  sqlc.arg(type),
  sqlc.arg(session_mode),
  sqlc.arg(runtime_type),
  sqlc.arg(runtime_metadata),
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
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.session_mode, s.runtime_type, s.runtime_metadata, s.title, s.metadata,
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
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.session_mode, s.runtime_type, s.runtime_metadata, s.title, s.metadata,
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
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.session_mode, s.runtime_type, s.runtime_metadata, s.title, s.metadata,
  s.parent_session_id, s.created_by_user_id, s.created_at, s.updated_at, s.deleted_at,
  r.metadata AS route_metadata,
  r.conversation_type AS route_conversation_type
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.deleted_at IS NULL
  AND s.type = ANY(sqlc.arg(types)::text[])
  AND (
    NOT sqlc.arg(use_parent_session)::bool
    OR s.parent_session_id = sqlc.narg(parent_session_id)::uuid
  )
  AND (
    NOT sqlc.arg(use_cursor)::bool
    OR (s.updated_at, s.id) < (sqlc.arg(cursor_updated_at)::timestamptz, sqlc.arg(cursor_id)::uuid)
  )
ORDER BY s.updated_at DESC, s.id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: ListSessionsByBotAndCreatedByUserPaged :many
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.session_mode, s.runtime_type, s.runtime_metadata, s.title, s.metadata,
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
    NOT sqlc.arg(use_parent_session)::bool
    OR s.parent_session_id = sqlc.narg(parent_session_id)::uuid
  )
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
SET type = sqlc.arg(type),
    session_mode = sqlc.arg(session_mode),
    runtime_type = sqlc.arg(runtime_type),
    runtime_metadata = sqlc.arg(runtime_metadata),
    metadata = sqlc.arg(metadata),
    updated_at = now()
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

-- name: GetSessionDiscussCursor :one
SELECT *
FROM bot_session_discuss_cursors
WHERE session_id = sqlc.arg(session_id)
  AND scope_key = sqlc.arg(scope_key);

-- name: ListSessionDiscussCursorsByBot :many
SELECT c.*
FROM bot_session_discuss_cursors c
JOIN bot_sessions s ON s.id = c.session_id
WHERE s.bot_id = sqlc.arg(bot_id)
ORDER BY c.updated_at ASC, c.session_id ASC, c.scope_key ASC;

-- name: DeleteSessionDiscussCursorsByBot :exec
DELETE FROM bot_session_discuss_cursors
WHERE session_id IN (
  SELECT id
  FROM bot_sessions
  WHERE bot_id = sqlc.arg(bot_id)
);

-- name: UpsertSessionDiscussCursor :one
INSERT INTO bot_session_discuss_cursors (
  session_id, scope_key, route_id, source, consumed_cursor
)
VALUES (
  sqlc.arg(session_id),
  sqlc.arg(scope_key),
  sqlc.narg(route_id)::uuid,
  sqlc.arg(source),
  sqlc.arg(consumed_cursor)
)
ON CONFLICT (session_id, scope_key) DO UPDATE
SET route_id = COALESCE(EXCLUDED.route_id, bot_session_discuss_cursors.route_id),
    source = EXCLUDED.source,
    consumed_cursor = GREATEST(bot_session_discuss_cursors.consumed_cursor, EXCLUDED.consumed_cursor),
    updated_at = now()
RETURNING *;

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
