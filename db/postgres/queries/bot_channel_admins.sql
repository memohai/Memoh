-- name: ListBotChannelAdmins :many
SELECT
  a.id,
  a.bot_id,
  a.channel_identity_id,
  a.granted,
  a.created_by_user_id,
  a.created_at,
  a.updated_at,
  ci.channel_type,
  ci.channel_subject_id,
  ci.display_name AS channel_identity_display_name,
  ci.avatar_url AS channel_identity_avatar_url
FROM bot_channel_admins a
INNER JOIN bots bot_scope ON bot_scope.id = a.bot_id AND bot_scope.team_id = a.team_id
LEFT JOIN channel_identities ci ON ci.id = a.channel_identity_id AND ci.team_id = a.team_id
WHERE a.bot_id = sqlc.arg(bot_id)
ORDER BY a.created_at DESC;

-- name: UpsertBotChannelAdmin :one
INSERT INTO bot_channel_admins (
  team_id,
  bot_id,
  channel_identity_id,
  granted,
  created_by_user_id
)
SELECT
  b.team_id,
  b.id,
  ci.id,
  sqlc.arg(granted),
  sqlc.narg(created_by_user_id)::uuid
FROM bots b
INNER JOIN channel_identities ci ON ci.id = sqlc.arg(channel_identity_id) AND ci.team_id = b.team_id
WHERE b.id = sqlc.arg(bot_id)
ON CONFLICT (team_id, bot_id, channel_identity_id) DO UPDATE
  SET granted = EXCLUDED.granted,
      updated_at = now()
RETURNING *;

-- name: DeleteBotChannelAdmin :exec
DELETE FROM bot_channel_admins
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
  AND channel_identity_id = sqlc.arg(channel_identity_id);

-- name: GetBotChannelAdmin :one
SELECT *
FROM bot_channel_admins
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
  AND channel_identity_id = sqlc.arg(channel_identity_id);
