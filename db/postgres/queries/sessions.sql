-- name: CreateSession :one
WITH input AS (
  SELECT
    sqlc.arg(bot_id)::uuid AS bot_id,
    sqlc.narg(route_id)::uuid AS route_id,
    sqlc.narg(channel_type)::text AS channel_type,
    sqlc.arg(type)::text AS session_type,
    sqlc.arg(session_mode)::text AS session_mode,
    sqlc.arg(runtime_type)::text AS runtime_type,
    sqlc.arg(runtime_metadata)::jsonb AS runtime_metadata,
    sqlc.arg(title)::text AS title,
    sqlc.arg(metadata)::jsonb AS metadata,
    sqlc.narg(default_head_turn_id)::uuid AS default_head_turn_id,
    sqlc.narg(forked_from_session_id)::uuid AS forked_from_session_id,
    sqlc.narg(forked_from_turn_id)::uuid AS forked_from_turn_id,
    sqlc.narg(parent_session_id)::uuid AS parent_session_id,
    sqlc.narg(created_by_user_id)::uuid AS created_by_user_id
)
INSERT INTO bot_sessions (
  bot_id, route_id, channel_type, type, session_mode, runtime_type, runtime_metadata, title, metadata,
  default_head_turn_id, forked_from_session_id, forked_from_turn_id,
  parent_session_id, created_by_user_id
)
SELECT
  bot_id,
  route_id,
  channel_type,
  session_type,
  session_mode,
  runtime_type,
  runtime_metadata,
  title,
  metadata,
  default_head_turn_id,
  forked_from_session_id,
  forked_from_turn_id,
  parent_session_id,
  created_by_user_id
FROM input
WHERE (
    default_head_turn_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns t
      WHERE t.id = input.default_head_turn_id
        AND t.bot_id = input.bot_id
    )
  )
  AND (
    forked_from_turn_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_turns t
      WHERE t.id = input.forked_from_turn_id
        AND t.bot_id = input.bot_id
    )
  )
  AND (
    forked_from_session_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = input.forked_from_session_id
        AND s.bot_id = input.bot_id
    )
  )
  AND (
    parent_session_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_sessions s
      WHERE s.id = input.parent_session_id
        AND s.bot_id = input.bot_id
    )
  )
RETURNING *;

-- name: GetSessionByID :one
SELECT *
FROM bot_sessions
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetSessionByIDIncludingDeleted :one
SELECT *
FROM bot_sessions
WHERE id = $1;

-- name: ListSessionsByBot :many
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.session_mode, s.runtime_type, s.runtime_metadata, s.title, s.metadata,
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
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.session_mode, s.runtime_type, s.runtime_metadata, s.title, s.metadata,
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

-- name: ListSessionsByBotPaged :many
-- Cursor uses (updated_at, id) so pages stay stable when many rows share an
-- updated_at. Callers always pass an explicit types filter; to opt out of
-- filtering, pass every known type.
SELECT
  s.id, s.bot_id, s.route_id, s.channel_type, s.type, s.session_mode, s.runtime_type, s.runtime_metadata, s.title, s.metadata,
  s.default_head_turn_id, s.forked_from_session_id, s.forked_from_turn_id,
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
  s.default_head_turn_id, s.forked_from_session_id, s.forked_from_turn_id,
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

-- name: UpdateSessionRestoredLinks :one
UPDATE bot_sessions
SET parent_session_id = sqlc.narg(parent_session_id)::uuid,
    forked_from_session_id = sqlc.narg(forked_from_session_id)::uuid,
    forked_from_turn_id = sqlc.narg(forked_from_turn_id)::uuid,
    default_head_turn_id = sqlc.narg(default_head_turn_id)::uuid,
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
SET updated_at = now()
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
WITH valid_new AS (
  SELECT existing.session_id, existing.bot_id, t.id AS new_head_turn_id
  FROM bot_session_turn_heads existing
  JOIN bot_history_turns t
    ON t.id = sqlc.arg(new_head_turn_id)
   AND t.bot_id = existing.bot_id
  WHERE existing.session_id = sqlc.arg(target_session_id)
    AND existing.head_turn_id = sqlc.arg(old_head_turn_id)
),
removed AS (
  DELETE FROM bot_session_turn_heads h
  WHERE h.session_id = sqlc.arg(target_session_id)
    AND h.head_turn_id = sqlc.arg(old_head_turn_id)
    AND EXISTS (
      SELECT 1
      FROM valid_new
      WHERE valid_new.session_id = h.session_id
        AND valid_new.bot_id = h.bot_id
    )
  RETURNING h.session_id, h.bot_id
)
INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id)
SELECT removed.session_id, valid_new.new_head_turn_id, removed.bot_id
FROM removed
JOIN valid_new
  ON valid_new.session_id = removed.session_id
 AND valid_new.bot_id = removed.bot_id
ON CONFLICT (session_id, head_turn_id) DO UPDATE
SET updated_at = now()
RETURNING *;

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
WITH input AS (
  SELECT
    sqlc.arg(id)::uuid AS id,
    sqlc.narg(default_head_turn_id)::uuid AS default_head_turn_id
)
UPDATE bot_sessions s
SET default_head_turn_id = input.default_head_turn_id,
    updated_at = now()
FROM input
WHERE s.id = input.id
  AND s.deleted_at IS NULL
  AND (
    input.default_head_turn_id IS NULL
    OR EXISTS (
      SELECT 1
      FROM bot_session_turn_heads h
      WHERE h.session_id = s.id
        AND h.bot_id = s.bot_id
        AND h.head_turn_id = input.default_head_turn_id
    )
  )
RETURNING s.*;

-- name: UpdateSessionDefaultHeadTurnIfValid :one
UPDATE bot_sessions
SET default_head_turn_id = sqlc.arg(default_head_turn_id),
    updated_at = now()
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

-- name: ClearSessionTurnPointersByBot :exec
UPDATE bot_sessions
SET default_head_turn_id = NULL,
    forked_from_session_id = NULL,
    forked_from_turn_id = NULL,
    updated_at = now()
WHERE bot_id = sqlc.arg(bot_id);
