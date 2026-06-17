-- name: CreateFetchProvider :one
INSERT INTO fetch_providers (id, name, provider, config, enable)
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

-- name: GetFetchProviderByID :one
SELECT * FROM fetch_providers WHERE id = sqlc.arg(id);

-- name: GetFetchProviderByName :one
SELECT * FROM fetch_providers WHERE name = sqlc.arg(name);

-- name: ListFetchProviders :many
SELECT * FROM fetch_providers
ORDER BY created_at DESC;

-- name: ListFetchProvidersByProvider :many
SELECT * FROM fetch_providers
WHERE provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateFetchProvider :one
UPDATE fetch_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  enable = sqlc.arg(enable),
  updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteFetchProvider :exec
DELETE FROM fetch_providers WHERE id = sqlc.arg(id);
