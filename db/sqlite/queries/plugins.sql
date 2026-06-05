-- name: CreateBotPluginInstallation :one
INSERT INTO bot_plugin_installations (
  id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(plugin_id),
  sqlc.arg(plugin_name),
  sqlc.arg(version),
  sqlc.arg(status),
  sqlc.arg(enabled),
  sqlc.arg(config),
  sqlc.arg(metadata),
  sqlc.arg(manifest)
)
ON CONFLICT (bot_id, plugin_id)
DO UPDATE SET plugin_name = EXCLUDED.plugin_name,
              version = EXCLUDED.version,
              status = EXCLUDED.status,
              enabled = EXCLUDED.enabled,
              config = EXCLUDED.config,
              metadata = EXCLUDED.metadata,
              manifest = EXCLUDED.manifest,
              updated_at = CURRENT_TIMESTAMP
RETURNING id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at;

-- name: GetBotPluginInstallationByID :one
SELECT id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at
FROM bot_plugin_installations
WHERE bot_id = sqlc.arg(bot_id) AND id = sqlc.arg(id)
LIMIT 1;

-- name: ListBotPluginInstallations :many
SELECT id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at
FROM bot_plugin_installations
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY installed_at DESC;

-- name: UpdateBotPluginInstallationStatus :one
UPDATE bot_plugin_installations
SET status = sqlc.arg(status),
    enabled = sqlc.arg(enabled),
    updated_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id) AND id = sqlc.arg(id)
RETURNING id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at;

-- name: DeleteBotPluginInstallation :exec
DELETE FROM bot_plugin_installations
WHERE bot_id = sqlc.arg(bot_id) AND id = sqlc.arg(id);

-- name: UpsertBotPluginResource :one
INSERT INTO bot_plugin_resources (
  id, installation_id, resource_type, resource_key, resource_id, status, metadata
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(installation_id),
  sqlc.arg(resource_type),
  sqlc.arg(resource_key),
  sqlc.arg(resource_id),
  sqlc.arg(status),
  sqlc.arg(metadata)
)
ON CONFLICT (installation_id, resource_type, resource_key)
DO UPDATE SET resource_id = EXCLUDED.resource_id,
              status = EXCLUDED.status,
              metadata = EXCLUDED.metadata,
              updated_at = CURRENT_TIMESTAMP
RETURNING id, installation_id, resource_type, resource_key, resource_id, status, metadata, created_at, updated_at;

-- name: ListBotPluginResources :many
SELECT id, installation_id, resource_type, resource_key, resource_id, status, metadata, created_at, updated_at
FROM bot_plugin_resources
WHERE installation_id = sqlc.arg(installation_id)
ORDER BY resource_type ASC, resource_key ASC;

-- name: DeleteBotPluginResources :exec
DELETE FROM bot_plugin_resources
WHERE installation_id = sqlc.arg(installation_id);

