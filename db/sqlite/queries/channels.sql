-- name: DeleteBotChannelConfig :exec
DELETE FROM bot_channel_configs
WHERE bot_id = sqlc.arg(bot_id) AND channel_type = sqlc.arg(channel_type);

-- name: GetBotChannelConfig :one
SELECT id, bot_id, channel_type, credentials, external_identity, self_identity, routing, capabilities, disabled, verified_at, created_at, updated_at
FROM bot_channel_configs
WHERE bot_id = sqlc.arg(bot_id) AND channel_type = sqlc.arg(channel_type)
LIMIT 1;

-- name: GetBotChannelConfigByExternalIdentity :one
SELECT id, bot_id, channel_type, credentials, external_identity, self_identity, routing, capabilities, disabled, verified_at, created_at, updated_at
FROM bot_channel_configs
WHERE channel_type = sqlc.arg(channel_type) AND external_identity = sqlc.arg(external_identity)
LIMIT 1;

-- name: UpsertBotChannelConfig :one
INSERT INTO bot_channel_configs (
  id, bot_id, channel_type, credentials, external_identity, self_identity, routing, capabilities, disabled, verified_at
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(channel_type),
  sqlc.arg(credentials),
  sqlc.arg(external_identity),
  sqlc.arg(self_identity),
  sqlc.arg(routing),
  sqlc.arg(capabilities),
  sqlc.arg(disabled),
  sqlc.arg(verified_at)
)
ON CONFLICT (bot_id, channel_type)
DO UPDATE SET
  credentials = EXCLUDED.credentials,
  external_identity = EXCLUDED.external_identity,
  self_identity = EXCLUDED.self_identity,
  routing = EXCLUDED.routing,
  capabilities = EXCLUDED.capabilities,
  disabled = EXCLUDED.disabled,
  verified_at = EXCLUDED.verified_at,
  updated_at = CURRENT_TIMESTAMP
RETURNING id, bot_id, channel_type, credentials, external_identity, self_identity, routing, capabilities, disabled, verified_at, created_at, updated_at;

-- name: UpdateBotChannelConfigDisabled :one
UPDATE bot_channel_configs
SET
  disabled = sqlc.arg(disabled),
  updated_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id) AND channel_type = sqlc.arg(channel_type)
RETURNING id, bot_id, channel_type, credentials, external_identity, self_identity, routing, capabilities, disabled, verified_at, created_at, updated_at;

-- name: SaveMatrixSyncSinceToken :execrows
UPDATE bot_channel_configs
SET routing = json_set(COALESCE(routing, '{}'), '$._matrix.since_token', sqlc.arg(since_token)),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: ListBotChannelConfigsByType :many
SELECT id, bot_id, channel_type, credentials, external_identity, self_identity, routing, capabilities, disabled, verified_at, created_at, updated_at
FROM bot_channel_configs
WHERE channel_type = sqlc.arg(channel_type)
ORDER BY created_at DESC;

-- name: GetUserChannelBinding :one
SELECT id, user_id, channel_type, config, created_at, updated_at
FROM iam_user_channel_bindings
WHERE user_id = sqlc.arg(user_id) AND channel_type = sqlc.arg(channel_type)
LIMIT 1;

-- name: UpsertUserChannelBinding :one
INSERT INTO iam_user_channel_bindings (id, user_id, channel_type, config)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(user_id),
  sqlc.arg(channel_type),
  sqlc.arg(config)
)
ON CONFLICT (user_id, channel_type)
DO UPDATE SET
  config = EXCLUDED.config,
  updated_at = CURRENT_TIMESTAMP
RETURNING id, user_id, channel_type, config, created_at, updated_at;

-- name: ListUserChannelBindingsByPlatform :many
SELECT id, user_id, channel_type, config, created_at, updated_at
FROM iam_user_channel_bindings
WHERE channel_type = sqlc.arg(channel_type)
ORDER BY created_at DESC;
