-- name: CreateSearchProvider :one
INSERT INTO search_providers (name, provider, config, enable)
VALUES (
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config),
  sqlc.arg(enable)
)
RETURNING *;

-- name: GetSearchProviderByID :one
SELECT * FROM search_providers WHERE tenant_id = app.current_tenant_id() AND id = sqlc.arg(id);

-- name: GetSearchProviderByName :one
SELECT * FROM search_providers WHERE tenant_id = app.current_tenant_id() AND name = sqlc.arg(name);

-- name: ListSearchProviders :many
SELECT * FROM search_providers
WHERE tenant_id = app.current_tenant_id()
ORDER BY created_at DESC;

-- name: ListSearchProvidersByProvider :many
SELECT * FROM search_providers
WHERE tenant_id = app.current_tenant_id() AND provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateSearchProvider :one
UPDATE search_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  enable = sqlc.arg(enable),
  updated_at = now()
WHERE tenant_id = app.current_tenant_id() AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteSearchProvider :exec
DELETE FROM search_providers WHERE tenant_id = app.current_tenant_id() AND id = sqlc.arg(id);
