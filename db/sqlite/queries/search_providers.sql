-- name: CreateSearchProvider :one
INSERT INTO search_providers (id, name, provider, config, enable)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config),
  sqlc.arg(enable)
)
RETURNING *;

-- name: GetSearchProviderByID :one
SELECT * FROM search_providers WHERE id = sqlc.arg(id);

-- name: GetSearchProviderByName :one
SELECT * FROM search_providers WHERE name = sqlc.arg(name);

-- name: ListSearchProviders :many
SELECT * FROM search_providers
ORDER BY created_at DESC;

-- name: ListSearchProvidersByProvider :many
SELECT * FROM search_providers
WHERE provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateSearchProvider :one
UPDATE search_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  enable = sqlc.arg(enable),
  updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteSearchProvider :exec
DELETE FROM search_providers WHERE id = sqlc.arg(id);
