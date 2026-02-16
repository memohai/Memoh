-- name: CreateStorageProvider :one
INSERT INTO storage_providers (name, provider, config)
VALUES (sqlc.arg(name), sqlc.arg(provider), sqlc.arg(config))
RETURNING *;

-- name: GetStorageProviderByID :one
SELECT * FROM storage_providers WHERE id = sqlc.arg(id);

-- name: GetStorageProviderByName :one
SELECT * FROM storage_providers WHERE name = sqlc.arg(name);

-- name: ListStorageProviders :many
SELECT * FROM storage_providers ORDER BY created_at DESC;

-- name: UpsertBotStorageBinding :one
INSERT INTO bot_storage_bindings (bot_id, storage_provider_id, base_path)
VALUES (sqlc.arg(bot_id), sqlc.arg(storage_provider_id), sqlc.arg(base_path))
ON CONFLICT (bot_id) DO UPDATE SET
  storage_provider_id = EXCLUDED.storage_provider_id,
  base_path = EXCLUDED.base_path,
  updated_at = now()
RETURNING *;

-- name: GetBotStorageBinding :one
SELECT * FROM bot_storage_bindings WHERE bot_id = sqlc.arg(bot_id);

-- name: CreateMediaAsset :one
INSERT INTO media_assets (
  bot_id, storage_provider_id, content_hash, media_type, mime,
  size_bytes, storage_key, original_name, width, height, duration_ms, metadata
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(storage_provider_id)::uuid,
  sqlc.arg(content_hash),
  sqlc.arg(media_type),
  sqlc.arg(mime),
  sqlc.arg(size_bytes),
  sqlc.arg(storage_key),
  sqlc.narg(original_name)::text,
  sqlc.narg(width)::integer,
  sqlc.narg(height)::integer,
  sqlc.narg(duration_ms)::bigint,
  sqlc.arg(metadata)
)
ON CONFLICT (bot_id, content_hash) DO UPDATE SET
  bot_id = media_assets.bot_id
RETURNING *;

-- name: GetMediaAssetByID :one
SELECT * FROM media_assets WHERE id = sqlc.arg(id);

-- name: GetMediaAssetByHash :one
SELECT * FROM media_assets
WHERE bot_id = sqlc.arg(bot_id) AND content_hash = sqlc.arg(content_hash);

-- name: ListMediaAssetsByBotID :many
SELECT * FROM media_assets
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY created_at DESC;

-- name: DeleteMediaAsset :exec
DELETE FROM media_assets WHERE id = sqlc.arg(id);

-- name: CreateMessageAsset :one
INSERT INTO bot_history_message_assets (message_id, asset_id, role, ordinal)
VALUES (sqlc.arg(message_id), sqlc.arg(asset_id), sqlc.arg(role), sqlc.arg(ordinal))
ON CONFLICT (message_id, asset_id) DO UPDATE SET
  role = EXCLUDED.role,
  ordinal = EXCLUDED.ordinal
RETURNING *;

-- name: ListMessageAssets :many
SELECT
  ma.id AS rel_id,
  ma.message_id,
  ma.asset_id,
  ma.role,
  ma.ordinal,
  a.media_type,
  a.mime,
  a.size_bytes,
  a.storage_key,
  a.original_name,
  a.width,
  a.height,
  a.duration_ms,
  a.metadata AS asset_metadata
FROM bot_history_message_assets ma
JOIN media_assets a ON a.id = ma.asset_id
WHERE ma.message_id = sqlc.arg(message_id)
ORDER BY ma.ordinal ASC;

-- name: ListMessageAssetsBatch :many
SELECT
  ma.id AS rel_id,
  ma.message_id,
  ma.asset_id,
  ma.role,
  ma.ordinal,
  a.media_type,
  a.mime,
  a.size_bytes,
  a.storage_key,
  a.original_name,
  a.width,
  a.height,
  a.duration_ms,
  a.metadata AS asset_metadata
FROM bot_history_message_assets ma
JOIN media_assets a ON a.id = ma.asset_id
WHERE ma.message_id = ANY(sqlc.arg(message_ids)::uuid[])
ORDER BY ma.message_id, ma.ordinal ASC;

-- name: DeleteMessageAssets :exec
DELETE FROM bot_history_message_assets WHERE message_id = sqlc.arg(message_id);
