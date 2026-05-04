-- name: ListMemoryProviders :many
SELECT * FROM memory_providers ORDER BY created_at ASC;

-- name: GetMemoryProviderByID :one
SELECT * FROM memory_providers WHERE id = sqlc.arg(id);

-- name: GetDefaultMemoryProvider :one
SELECT * FROM memory_providers WHERE is_default = true LIMIT 1;

-- name: CreateMemoryProvider :one
INSERT INTO memory_providers (id, name, provider, config, is_default)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config),
  sqlc.arg(is_default)
)
RETURNING *;

-- name: UpdateMemoryProvider :one
UPDATE memory_providers
SET name = sqlc.arg(name),
    config = sqlc.arg(config),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteMemoryProvider :exec
DELETE FROM memory_providers WHERE id = sqlc.arg(id);

-- name: CountMemoryProvidersByDefault :one
SELECT COUNT(*) FROM memory_providers WHERE is_default = true;
