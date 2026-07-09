-- name: CreateSearchProvider :one
INSERT INTO search_providers (team_id, name, provider, config, enable)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config),
  sqlc.arg(enable)
)
RETURNING *;

-- name: GetSearchProviderByID :one
SELECT * FROM search_providers WHERE id = sqlc.arg(id);

-- name: GetSearchProviderByIDForTeam :one
SELECT * FROM search_providers
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetSearchProviderByName :one
SELECT * FROM search_providers WHERE name = sqlc.arg(name);

-- name: GetSearchProviderByNameForTeam :one
SELECT * FROM search_providers
WHERE team_id = sqlc.arg(team_id)
  AND name = sqlc.arg(name);

-- name: ListSearchProviders :many
SELECT * FROM search_providers
ORDER BY created_at DESC;

-- name: ListSearchProvidersForTeam :many
SELECT * FROM search_providers
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC;

-- name: ListSearchProvidersByProvider :many
SELECT * FROM search_providers
WHERE provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: ListSearchProvidersByProviderForTeam :many
SELECT * FROM search_providers
WHERE team_id = sqlc.arg(team_id)
  AND provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateSearchProvider :one
UPDATE search_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  enable = sqlc.arg(enable),
  updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: DeleteSearchProvider :exec
DELETE FROM search_providers WHERE id = sqlc.arg(id);

-- name: DeleteSearchProviderForTeam :exec
DELETE FROM search_providers
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);
