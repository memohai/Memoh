-- name: CreateBotPluginInstallation :one
INSERT INTO bot_plugin_installations (
  bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (bot_id, plugin_id)
DO UPDATE SET plugin_name = EXCLUDED.plugin_name,
              version = EXCLUDED.version,
              status = EXCLUDED.status,
              enabled = EXCLUDED.enabled,
              config = EXCLUDED.config,
              metadata = EXCLUDED.metadata,
              manifest = EXCLUDED.manifest,
              updated_at = now()
RETURNING id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at;

-- name: GetBotPluginInstallationByID :one
SELECT id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at
FROM bot_plugin_installations
WHERE bot_id = $1 AND id = $2
LIMIT 1;

-- name: ListBotPluginInstallations :many
SELECT id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at
FROM bot_plugin_installations
WHERE bot_id = $1
ORDER BY installed_at DESC;

-- name: UpdateBotPluginInstallationStatus :one
UPDATE bot_plugin_installations
SET status = $3,
    enabled = $4,
    updated_at = now()
WHERE bot_id = $1 AND id = $2
RETURNING id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest, installed_at, updated_at;

-- name: DeleteBotPluginInstallation :exec
DELETE FROM bot_plugin_installations
WHERE bot_id = $1 AND id = $2;

-- name: UpsertBotPluginResource :one
INSERT INTO bot_plugin_resources (
  installation_id, resource_type, resource_key, resource_id, status, metadata
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (installation_id, resource_type, resource_key)
DO UPDATE SET resource_id = EXCLUDED.resource_id,
              status = EXCLUDED.status,
              metadata = EXCLUDED.metadata,
              updated_at = now()
RETURNING id, installation_id, resource_type, resource_key, resource_id, status, metadata, created_at, updated_at;

-- name: ListBotPluginResources :many
SELECT id, installation_id, resource_type, resource_key, resource_id, status, metadata, created_at, updated_at
FROM bot_plugin_resources
WHERE installation_id = $1
ORDER BY resource_type ASC, resource_key ASC;

-- name: DeleteBotPluginResources :exec
DELETE FROM bot_plugin_resources
WHERE installation_id = $1;
