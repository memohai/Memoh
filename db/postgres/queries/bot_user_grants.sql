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
  AND g.team_id = sqlc.arg(team_id)::uuid
ORDER BY g.subject_type DESC, g.created_at ASC;

-- name: GetBotUserGrantByID :one
SELECT *
FROM bot_user_grants
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: ListBotUserGrantsForUser :many
SELECT id, bot_id, subject_type, user_id, permissions
FROM bot_user_grants
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid
  AND (
    subject_type = 'everyone'
    OR (subject_type = 'user' AND user_id = sqlc.narg(user_id)::uuid)
  );

-- name: CreateBotUserGrant :one
INSERT INTO bot_user_grants (team_id, bot_id, subject_type, user_id, permissions, created_by_user_id)
VALUES (
  sqlc.arg(team_id)::uuid,
  sqlc.arg(bot_id),
  sqlc.arg(subject_type),
  sqlc.narg(user_id)::uuid,
  sqlc.arg(permissions),
  sqlc.narg(created_by_user_id)::uuid
)
RETURNING *;

-- name: UpdateBotUserGrantPermissions :one
UPDATE bot_user_grants
SET permissions = sqlc.arg(permissions),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid
RETURNING *;

-- name: DeleteBotUserGrantByID :exec
DELETE FROM bot_user_grants
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;
