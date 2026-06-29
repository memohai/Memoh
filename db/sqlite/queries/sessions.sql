-- name: CreateSession :one
INSERT INTO bot_sessions (
  id, bot_id, route_id, channel_type, type, title, metadata,
  default_head_turn_id, forked_from_session_id, forked_from_turn_id,
  parent_session_id, created_by_user_id
)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.narg(route_id),
  sqlc.narg(channel_type),
  sqlc.arg(type),
  sqlc.arg(title),
  sqlc.arg(metadata),
  sqlc.narg(default_head_turn_id),
  sqlc.narg(forked_from_session_id),
  sqlc.narg(forked_from_turn_id),
  sqlc.narg(parent_session_id),
  sqlc.narg(created_by_user_id)
WHERE (
    sqlc.narg(default_head_turn_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns t
      WHERE t.id = sqlc.narg(default_head_turn_id)
        AND t.bot_id = sqlc.arg(bot_id)
    )
  )
  AND (
    sqlc.narg(forked_from_turn_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns t
      WHERE t.id = sqlc.narg(forked_from_turn_id)
        AND t.bot_id = sqlc.arg(bot_id)
    )
  )
  AND (
    sqlc.narg(forked_from_session_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = sqlc.narg(forked_from_session_id)
        AND s.bot_id = sqlc.arg(bot_id)
    )
  )
  AND (
    sqlc.narg(parent_session_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = sqlc.narg(parent_session_id)
        AND s.bot_id = sqlc.arg(bot_id)
    )
  )
RETURNING *;

-- name: GetSessionByID :one
SELECT *
FROM bot_sessions
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: GetSessionByIDIncludingDeleted :one
SELECT *
FROM bot_sessions
WHERE id = sqlc.arg(id);

-- name: ListSessionsByBot :many
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.title, s.metadata,
  s.default_head_turn_id, s.forked_from_session_id, s.forked_from_turn_id,
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
  s.default_head_turn_id, s.forked_from_session_id, s.forked_from_turn_id,
  s.parent_session_id, s.created_by_user_id, s.created_at, s.updated_at, s.deleted_at,
  r.metadata AS route_metadata,
  r.conversation_type AS route_conversation_type
FROM bot_sessions s
LEFT JOIN bot_channel_routes r ON r.id = s.route_id
WHERE s.bot_id = sqlc.arg(bot_id)
  AND s.created_by_user_id = sqlc.arg(created_by_user_id)
  AND s.deleted_at IS NULL
ORDER BY s.updated_at DESC;

-- ListSessionsByBotPaged and ListSessionsByBotAndCreatedByUserPaged are not
-- generated for SQLite. The Postgres versions live in
-- db/postgres/queries/sessions.sql; the SQLite shim hand-rolls the query in
-- internal/db/sqlite/store/sessions_paged.go because sqlc-sqlite cannot mix
-- sqlc.slice with reused numbered placeholders without colliding bind indexes.



-- name: ListSessionsByRoute :many
SELECT *
FROM bot_sessions
WHERE route_id = sqlc.arg(route_id)
  AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: UpdateSessionTitle :one
UPDATE bot_sessions
SET title = sqlc.arg(title), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateSessionMetadata :one
UPDATE bot_sessions
SET metadata = sqlc.arg(metadata), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateSessionTypeAndMetadata :one
UPDATE bot_sessions
SET type = sqlc.arg(type), metadata = sqlc.arg(metadata), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateSessionRestoredLinks :one
UPDATE bot_sessions
SET parent_session_id = sqlc.narg(parent_session_id),
    forked_from_session_id = sqlc.narg(forked_from_session_id),
    forked_from_turn_id = sqlc.narg(forked_from_turn_id),
    default_head_turn_id = sqlc.narg(default_head_turn_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteSession :exec
UPDATE bot_sessions
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: TouchSession :exec
UPDATE bot_sessions
SET updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: CreateSessionTurnHead :one
INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id)
SELECT s.id, t.id, s.bot_id
FROM bot_sessions s
JOIN bot_history_turns t
  ON t.id = sqlc.arg(head_turn_id)
 AND t.bot_id = s.bot_id
WHERE s.id = sqlc.arg(session_id)
  AND s.deleted_at IS NULL
ON CONFLICT (session_id, head_turn_id) DO UPDATE
SET updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetSessionTurnHead :one
SELECT *
FROM bot_session_turn_heads
WHERE session_id = sqlc.arg(session_id)
  AND head_turn_id = sqlc.arg(head_turn_id);

-- name: ListSessionTurnHeads :many
SELECT *
FROM bot_session_turn_heads
WHERE session_id = sqlc.arg(session_id)
ORDER BY created_at ASC, head_turn_id ASC;

-- name: ReplaceSessionTurnHead :one
INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id)
SELECT existing.session_id, t.id, existing.bot_id
FROM bot_session_turn_heads existing
JOIN bot_history_turns t
  ON t.id = sqlc.arg(new_head_turn_id)
 AND t.bot_id = existing.bot_id
WHERE existing.session_id = sqlc.arg(target_session_id)
  AND existing.head_turn_id = sqlc.arg(old_head_turn_id)
  AND EXISTS (
  SELECT 1
  FROM bot_sessions s
  WHERE s.id = existing.session_id
    AND s.bot_id = existing.bot_id
    AND s.deleted_at IS NULL
)
ON CONFLICT (session_id, head_turn_id) DO UPDATE
SET updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: DeleteReplacedSessionTurnHead :exec
DELETE FROM bot_session_turn_heads
WHERE session_id = sqlc.arg(target_session_id)
  AND head_turn_id = sqlc.arg(old_head_turn_id)
  AND head_turn_id != sqlc.arg(new_head_turn_id);

-- name: DeleteSessionTurnHeads :exec
DELETE FROM bot_session_turn_heads
WHERE session_id = sqlc.arg(session_id);

-- name: DeleteSessionTurnHeadsByBot :exec
DELETE FROM bot_session_turn_heads
WHERE session_id IN (
  SELECT s.id
  FROM bot_sessions s
  WHERE s.bot_id = sqlc.arg(bot_id)
);

-- name: UpdateSessionDefaultHeadTurn :one
UPDATE bot_sessions
SET default_head_turn_id = sqlc.narg(default_head_turn_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
  AND (
    sqlc.narg(default_head_turn_id) IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_session_turn_heads h
      WHERE h.session_id = bot_sessions.id
        AND h.bot_id = bot_sessions.bot_id
        AND h.head_turn_id = sqlc.narg(default_head_turn_id)
    )
  )
RETURNING *;

-- name: UpdateSessionDefaultHeadTurnIfValid :one
UPDATE bot_sessions
SET default_head_turn_id = sqlc.arg(default_head_turn_id),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM bot_session_turn_heads h
    WHERE h.session_id = bot_sessions.id
      AND h.bot_id = bot_sessions.bot_id
      AND h.head_turn_id = sqlc.arg(default_head_turn_id)
  )
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
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id) AND deleted_at IS NULL;

-- name: ClearSessionTurnPointersByBot :exec
UPDATE bot_sessions
SET default_head_turn_id = NULL,
    forked_from_session_id = NULL,
    forked_from_turn_id = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id);
