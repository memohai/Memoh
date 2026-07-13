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
LEFT JOIN channel_identities ci ON ci.id = a.channel_identity_id AND ci.tenant_id = app.current_tenant_id()
WHERE a.tenant_id = app.current_tenant_id() AND a.bot_id = $1
ORDER BY a.created_at DESC;

-- name: UpsertBotChannelAdmin :one
INSERT INTO bot_channel_admins (
  bot_id,
  channel_identity_id,
  granted,
  created_by_user_id
)
VALUES (
  $1,
  $2,
  $3,
  sqlc.narg(created_by_user_id)::uuid
)
ON CONFLICT (tenant_id, bot_id, channel_identity_id) DO UPDATE
  SET granted = EXCLUDED.granted,
      updated_at = now()
RETURNING *;

-- name: DeleteBotChannelAdmin :exec
DELETE FROM bot_channel_admins
WHERE tenant_id = app.current_tenant_id() AND bot_id = $1 AND channel_identity_id = $2;

-- name: GetBotChannelAdmin :one
SELECT id, bot_id, channel_identity_id, granted, created_by_user_id, created_at, updated_at, tenant_id
FROM bot_channel_admins
WHERE tenant_id = app.current_tenant_id() AND bot_id = $1 AND channel_identity_id = $2;
