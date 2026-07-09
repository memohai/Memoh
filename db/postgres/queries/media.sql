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
INSERT INTO bot_storage_bindings (team_id, bot_id, storage_provider_id, base_path)
SELECT b.team_id, b.id, sqlc.arg(storage_provider_id), sqlc.arg(base_path)
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)
ON CONFLICT (team_id, bot_id) DO UPDATE SET
  storage_provider_id = EXCLUDED.storage_provider_id,
  base_path = EXCLUDED.base_path,
  updated_at = now()
WHERE EXISTS (
  SELECT 1
  FROM bots b
  WHERE b.id = bot_storage_bindings.bot_id
    AND b.team_id = sqlc.arg(team_id)
)
RETURNING *;

-- name: GetBotStorageBinding :one
SELECT bsb.*
FROM bot_storage_bindings bsb
JOIN bots b ON b.id = bsb.bot_id
WHERE bsb.bot_id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id);

-- name: CreateMessageAsset :one
INSERT INTO bot_history_message_assets (team_id, message_id, role, ordinal, content_hash, name, metadata)
SELECT
  b.team_id,
  m.id,
  sqlc.arg(role),
  sqlc.arg(ordinal),
  sqlc.arg(content_hash),
  sqlc.arg(name),
  sqlc.arg(metadata)
FROM bot_history_messages m
JOIN bots b ON b.id = m.bot_id
WHERE m.id = sqlc.arg(message_id)
  AND b.team_id = sqlc.arg(team_id)
ON CONFLICT (team_id, message_id, content_hash) DO UPDATE SET
  role = EXCLUDED.role,
  ordinal = EXCLUDED.ordinal,
  name = EXCLUDED.name,
  metadata = EXCLUDED.metadata
RETURNING *;

-- name: ListMessageAssets :many
SELECT a.id AS rel_id, a.message_id, a.role, a.ordinal, a.content_hash, a.name, a.metadata
FROM bot_history_message_assets a
JOIN bot_history_messages m ON m.id = a.message_id
JOIN bots b ON b.id = m.bot_id
WHERE a.message_id = sqlc.arg(message_id)
  AND b.team_id = sqlc.arg(team_id)
ORDER BY a.ordinal ASC;

-- name: ListMessageAssetsBatch :many
SELECT a.id AS rel_id, a.message_id, a.role, a.ordinal, a.content_hash, a.name, a.metadata
FROM bot_history_message_assets a
JOIN bot_history_messages m ON m.id = a.message_id
JOIN bots b ON b.id = m.bot_id
WHERE a.message_id = ANY(sqlc.arg(message_ids)::uuid[])
  AND b.team_id = sqlc.arg(team_id)
ORDER BY a.message_id, a.ordinal ASC;

-- name: CountMessageAssetsByBot :one
SELECT COUNT(*)
FROM bot_history_message_assets a
JOIN bot_history_messages m ON m.id = a.message_id
JOIN bots b ON b.id = m.bot_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id);

-- name: DeleteMessageAssets :exec
DELETE FROM bot_history_message_assets AS a
USING bot_history_messages m, bots b
WHERE a.message_id = sqlc.arg(message_id)
  AND m.id = a.message_id
  AND b.id = m.bot_id
  AND b.team_id = sqlc.arg(team_id);
