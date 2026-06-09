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
LEFT JOIN channel_identities ci ON ci.id = a.channel_identity_id
WHERE a.bot_id = sqlc.arg(bot_id)
ORDER BY a.created_at DESC;

-- name: UpsertBotChannelAdmin :one
INSERT INTO bot_channel_admins (id, bot_id, channel_identity_id, granted, created_by_user_id)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(channel_identity_id),
  sqlc.arg(granted),
  sqlc.narg(created_by_user_id)
)
ON CONFLICT (bot_id, channel_identity_id) DO UPDATE
  SET granted = excluded.granted,
      updated_at = CURRENT_TIMESTAMP
RETURNING id, bot_id, channel_identity_id, granted, created_by_user_id, created_at, updated_at;

-- name: DeleteBotChannelAdmin :exec
DELETE FROM bot_channel_admins
WHERE bot_id = sqlc.arg(bot_id) AND channel_identity_id = sqlc.arg(channel_identity_id);

-- name: GetBotChannelAdmin :one
SELECT id, bot_id, channel_identity_id, granted, created_by_user_id, created_at, updated_at
FROM bot_channel_admins
WHERE bot_id = sqlc.arg(bot_id) AND channel_identity_id = sqlc.arg(channel_identity_id);
