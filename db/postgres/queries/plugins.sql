-- name: CreateBotPluginInstallation :one
INSERT INTO bot_plugin_installations (
  team_id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest
)
SELECT b.team_id, b.id, sqlc.arg(plugin_id), sqlc.arg(plugin_name), sqlc.arg(version), sqlc.arg(status), sqlc.arg(enabled), sqlc.arg(config), sqlc.arg(metadata), sqlc.arg(manifest)
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
ON CONFLICT (team_id, bot_id, plugin_id)
DO UPDATE SET plugin_name = EXCLUDED.plugin_name,
              version = EXCLUDED.version,
              status = EXCLUDED.status,
              enabled = EXCLUDED.enabled,
              config = EXCLUDED.config,
              metadata = EXCLUDED.metadata,
              manifest = EXCLUDED.manifest,
              updated_at = now()
RETURNING *;

-- name: GetBotPluginInstallationByID :one
SELECT *
FROM bot_plugin_installations
WHERE bot_id = sqlc.arg(bot_id)
  AND bot_plugin_installations.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
LIMIT 1;

-- name: ListBotPluginInstallations :many
SELECT *
FROM bot_plugin_installations
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
ORDER BY installed_at DESC;

-- name: UpdateBotPluginInstallationStatus :one
UPDATE bot_plugin_installations
SET status = sqlc.arg(status),
    enabled = sqlc.arg(enabled),
    updated_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND bot_plugin_installations.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
RETURNING *;

-- name: DeleteBotPluginInstallation :exec
DELETE FROM bot_plugin_installations
WHERE bot_id = sqlc.arg(bot_id)
  AND bot_plugin_installations.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id));

-- name: UpsertBotPluginResource :one
INSERT INTO bot_plugin_resources (
  team_id, installation_id, resource_type, resource_key, resource_id, status, metadata
)
SELECT i.team_id, i.id, sqlc.arg(resource_type), sqlc.arg(resource_key), sqlc.arg(resource_id), sqlc.arg(status), sqlc.arg(metadata)
FROM bot_plugin_installations i
WHERE i.id = sqlc.arg(installation_id)
ON CONFLICT (team_id, installation_id, resource_type, resource_key)
DO UPDATE SET resource_id = EXCLUDED.resource_id,
              status = EXCLUDED.status,
              metadata = EXCLUDED.metadata,
              updated_at = now()
RETURNING *;

-- name: ListBotPluginResources :many
SELECT *
FROM bot_plugin_resources
WHERE installation_id = sqlc.arg(installation_id)
  AND team_id = (SELECT i.team_id FROM bot_plugin_installations i WHERE i.id = sqlc.arg(installation_id))
ORDER BY resource_type ASC, resource_key ASC;

-- name: DeleteBotPluginResources :exec
DELETE FROM bot_plugin_resources
WHERE installation_id = sqlc.arg(installation_id)
  AND team_id = (SELECT i.team_id FROM bot_plugin_installations i WHERE i.id = sqlc.arg(installation_id));
