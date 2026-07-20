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
LEFT JOIN team_members membership
  ON membership.team_id = public.memoh_current_team_id()
 AND membership.user_id = g.user_id
LEFT JOIN users u ON u.id = membership.user_id
WHERE g.team_id = public.memoh_current_team_id() AND g.bot_id = $1
ORDER BY g.subject_type DESC, g.created_at ASC;

-- name: GetBotUserGrantByID :one
SELECT id, bot_id, subject_type, user_id, permissions, created_by_user_id, created_at, updated_at, team_id
FROM bot_user_grants
WHERE team_id = public.memoh_current_team_id() AND id = $1;

-- name: ListBotUserGrantsForUser :many
SELECT id, bot_id, subject_type, user_id, permissions
FROM bot_user_grants
WHERE team_id = public.memoh_current_team_id()
  AND bot_id = $1
  AND (
    subject_type = 'everyone'
    OR (subject_type = 'user' AND user_id = sqlc.narg(user_id)::uuid)
  );

-- name: CreateBotUserGrant :one
INSERT INTO bot_user_grants (bot_id, subject_type, user_id, permissions, created_by_user_id)
VALUES (
  $1,
  $2,
  sqlc.narg(user_id)::uuid,
  $3,
  sqlc.narg(created_by_user_id)::uuid
)
RETURNING id, bot_id, subject_type, user_id, permissions, created_by_user_id, created_at, updated_at, team_id;

-- name: UpdateBotUserGrantPermissions :one
UPDATE bot_user_grants
SET permissions = $2,
    updated_at = now()
WHERE team_id = public.memoh_current_team_id() AND id = $1
RETURNING id, bot_id, subject_type, user_id, permissions, created_by_user_id, created_at, updated_at, team_id;

-- name: DeleteBotUserGrantByID :exec
DELETE FROM bot_user_grants WHERE team_id = public.memoh_current_team_id() AND id = $1;
