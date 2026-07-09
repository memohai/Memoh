-- name: CreateFetchProvider :one
INSERT INTO fetch_providers (team_id, name, provider, config, enable)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config),
  sqlc.arg(enable)
)
RETURNING *;

-- name: GetFetchProviderByID :one
SELECT * FROM fetch_providers WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: GetFetchProviderByIDForTeam :one
SELECT * FROM fetch_providers
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetFetchProviderByName :one
SELECT * FROM fetch_providers WHERE name = sqlc.arg(name);

-- name: GetFetchProviderByNameForTeam :one
SELECT * FROM fetch_providers
WHERE team_id = sqlc.arg(team_id)
  AND name = sqlc.arg(name);

-- name: ListFetchProviders :many
SELECT * FROM fetch_providers
ORDER BY created_at DESC;

-- name: ListFetchProvidersForTeam :many
SELECT * FROM fetch_providers
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC;

-- name: ListFetchProvidersByProvider :many
SELECT * FROM fetch_providers
WHERE provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: ListFetchProvidersByProviderForTeam :many
SELECT * FROM fetch_providers
WHERE team_id = sqlc.arg(team_id)
  AND provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateFetchProvider :one
UPDATE fetch_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  enable = sqlc.arg(enable),
  updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: DeleteFetchProvider :exec
DELETE FROM fetch_providers WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: DeleteFetchProviderForTeam :exec
DELETE FROM fetch_providers
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);
