-- name: CreateStorageProvider :one
INSERT INTO storage_providers (id, name, provider, config)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetStorageProviderByID :one
SELECT * FROM storage_providers WHERE id = sqlc.arg(id);

-- name: GetStorageProviderByName :one
SELECT * FROM storage_providers WHERE name = sqlc.arg(name);

-- name: ListStorageProviders :many
SELECT * FROM storage_providers ORDER BY created_at DESC;

-- name: UpsertBotStorageBinding :one
INSERT INTO bot_storage_bindings (id, bot_id, storage_provider_id, base_path)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(storage_provider_id),
  sqlc.arg(base_path)
)
ON CONFLICT (bot_id) DO UPDATE SET
  storage_provider_id = EXCLUDED.storage_provider_id,
  base_path = EXCLUDED.base_path,
  updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetBotStorageBinding :one
SELECT * FROM bot_storage_bindings WHERE bot_id = sqlc.arg(bot_id);

-- name: CreateMessageAsset :one
INSERT INTO bot_history_message_assets (id, message_id, role, ordinal, content_hash, name, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(message_id),
  sqlc.arg(role),
  sqlc.arg(ordinal),
  sqlc.arg(content_hash),
  sqlc.arg(name),
  sqlc.arg(metadata)
)
ON CONFLICT (message_id, content_hash) DO UPDATE SET
  role = EXCLUDED.role,
  ordinal = EXCLUDED.ordinal,
  name = EXCLUDED.name,
  metadata = EXCLUDED.metadata
RETURNING *;

-- name: ListMessageAssets :many
SELECT id AS rel_id, message_id, role, ordinal, content_hash, name, metadata
FROM bot_history_message_assets
WHERE message_id = sqlc.arg(message_id)
ORDER BY ordinal ASC;

-- name: ListMessageAssetsBatch :many
SELECT id AS rel_id, message_id, role, ordinal, content_hash, name, metadata
FROM bot_history_message_assets
WHERE message_id IN (sqlc.slice(message_ids))
ORDER BY message_id, ordinal ASC;

-- name: DeleteMessageAssets :exec
DELETE FROM bot_history_message_assets WHERE message_id = sqlc.arg(message_id);
