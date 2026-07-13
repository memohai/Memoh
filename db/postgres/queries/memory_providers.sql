-- name: ListMemoryProviders :many
SELECT * FROM memory_providers WHERE tenant_id = app.current_tenant_id() ORDER BY created_at ASC;

-- name: GetMemoryProviderByID :one
SELECT * FROM memory_providers WHERE tenant_id = app.current_tenant_id() AND id = $1;

-- name: GetDefaultMemoryProvider :one
SELECT * FROM memory_providers WHERE tenant_id = app.current_tenant_id() AND is_default = true LIMIT 1;

-- name: CreateMemoryProvider :one
INSERT INTO memory_providers (name, provider, config, is_default)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateMemoryProvider :one
UPDATE memory_providers
SET name = $2,
    config = $3,
    updated_at = now()
WHERE tenant_id = app.current_tenant_id() AND id = $1
RETURNING *;

-- name: DeleteMemoryProvider :exec
DELETE FROM memory_providers WHERE tenant_id = app.current_tenant_id() AND id = $1;

-- name: CountMemoryProvidersByDefault :one
SELECT COUNT(*) FROM memory_providers WHERE tenant_id = app.current_tenant_id() AND is_default = true;
