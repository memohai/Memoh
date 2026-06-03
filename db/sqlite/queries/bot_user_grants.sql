-- name: ListBotUserGrants :many
SELECT
  g.id,
  g.bot_id,
  g.subject_type,
  g.user_id,
  g.permissions,
  g.created_by_user_id,
  g.created_at,
  g.updated_at,
  u.username AS user_username,
  u.display_name AS user_display_name,
  u.avatar_url AS user_avatar_url
FROM bot_user_grants g
LEFT JOIN users u ON u.id = g.user_id
WHERE g.bot_id = sqlc.arg(bot_id)
ORDER BY g.subject_type DESC, g.created_at ASC;

-- name: GetBotUserGrantByID :one
SELECT id, bot_id, subject_type, user_id, permissions, created_by_user_id, created_at, updated_at
FROM bot_user_grants
WHERE id = sqlc.arg(id);

-- name: ListBotUserGrantsForUser :many
SELECT id, bot_id, subject_type, user_id, permissions
FROM bot_user_grants
WHERE bot_id = sqlc.arg(bot_id)
  AND (
    subject_type = 'everyone'
    OR (subject_type = 'user' AND user_id = sqlc.narg(user_id))
  );

-- name: CreateBotUserGrant :one
INSERT INTO bot_user_grants (id, bot_id, subject_type, user_id, permissions, created_by_user_id)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(subject_type),
  sqlc.narg(user_id),
  sqlc.arg(permissions),
  sqlc.narg(created_by_user_id)
)
RETURNING id, bot_id, subject_type, user_id, permissions, created_by_user_id, created_at, updated_at;

-- name: UpdateBotUserGrantPermissions :one
UPDATE bot_user_grants
SET permissions = sqlc.arg(permissions),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, bot_id, subject_type, user_id, permissions, created_by_user_id, created_at, updated_at;

-- name: DeleteBotUserGrantByID :exec
DELETE FROM bot_user_grants WHERE id = sqlc.arg(id);
