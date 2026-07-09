-- name: DeleteBotChannelConfig :exec
DELETE FROM bot_channel_configs
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
  AND channel_type = sqlc.arg(channel_type);

-- name: GetBotChannelConfig :one
SELECT *
FROM bot_channel_configs
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
  AND channel_type = sqlc.arg(channel_type)
LIMIT 1;

-- name: GetBotChannelConfigByExternalIdentity :one
SELECT *
FROM bot_channel_configs
WHERE team_id = sqlc.arg(team_id)
  AND channel_type = sqlc.arg(channel_type)
  AND external_identity = sqlc.arg(external_identity)
LIMIT 1;

-- name: UpsertBotChannelConfig :one
INSERT INTO bot_channel_configs (
  team_id, bot_id, channel_type, credentials, external_identity, self_identity, routing, capabilities, disabled, verified_at
)
SELECT b.team_id, b.id, sqlc.arg(channel_type), sqlc.arg(credentials), sqlc.arg(external_identity), sqlc.arg(self_identity), sqlc.arg(routing), sqlc.arg(capabilities), sqlc.arg(disabled), sqlc.arg(verified_at)
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
ON CONFLICT (team_id, bot_id, channel_type)
DO UPDATE SET
  credentials = EXCLUDED.credentials,
  external_identity = EXCLUDED.external_identity,
  self_identity = EXCLUDED.self_identity,
  routing = EXCLUDED.routing,
  capabilities = EXCLUDED.capabilities,
  disabled = EXCLUDED.disabled,
  verified_at = EXCLUDED.verified_at,
  updated_at = now()
RETURNING *;

-- name: UpdateBotChannelConfigDisabled :one
UPDATE bot_channel_configs
SET
  disabled = sqlc.arg(disabled),
  updated_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
  AND channel_type = sqlc.arg(channel_type)
RETURNING *;

-- name: SaveMatrixSyncSinceToken :execrows
UPDATE bot_channel_configs
SET routing = COALESCE(routing, '{}'::jsonb) || jsonb_build_object(
  '_matrix',
  COALESCE(routing->'_matrix', '{}'::jsonb) || jsonb_build_object('since_token', sqlc.arg(since_token)::text)
)
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: ListBotChannelConfigsByType :many
-- Process-wide channel refresh / inbound webhook routing: intentionally spans
-- all teams (no team_id filter) so every team's adapters connect after a
-- restart and inbound webhooks resolve regardless of tenant. Each row carries
-- team_id for downstream per-config work.
SELECT *
FROM bot_channel_configs
WHERE channel_type = sqlc.arg(channel_type)
ORDER BY created_at DESC;

-- name: GetUserChannelBinding :one
SELECT *
FROM user_channel_bindings
WHERE team_id = sqlc.arg(team_id)
  AND user_id = sqlc.arg(user_id)
  AND channel_type = sqlc.arg(channel_type)
LIMIT 1;

-- name: UpsertUserChannelBinding :one
INSERT INTO user_channel_bindings (team_id, user_id, channel_type, config)
VALUES (sqlc.arg(team_id), sqlc.arg(user_id), sqlc.arg(channel_type), sqlc.arg(config))
ON CONFLICT (team_id, user_id, channel_type)
DO UPDATE SET
  config = EXCLUDED.config,
  updated_at = now()
RETURNING *;

-- name: ListUserChannelBindingsByPlatform :many
SELECT *
FROM user_channel_bindings
WHERE team_id = sqlc.arg(team_id)
  AND channel_type = sqlc.arg(channel_type)
ORDER BY created_at DESC;
