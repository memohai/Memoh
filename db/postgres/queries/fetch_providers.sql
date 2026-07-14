-- name: CreateFetchProvider :one
INSERT INTO fetch_providers (name, provider, config, enable)
VALUES (
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config),
  sqlc.arg(enable)
)
RETURNING *;

-- name: GetFetchProviderByID :one
SELECT * FROM fetch_providers WHERE team_id = app.current_team_id() AND id = sqlc.arg(id);

-- name: GetFetchProviderByName :one
SELECT * FROM fetch_providers WHERE team_id = app.current_team_id() AND name = sqlc.arg(name);

-- name: ListFetchProviders :many
SELECT * FROM fetch_providers
WHERE team_id = app.current_team_id()
ORDER BY created_at DESC;

-- name: ListFetchProvidersByProvider :many
SELECT * FROM fetch_providers
WHERE team_id = app.current_team_id() AND provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateFetchProvider :one
UPDATE fetch_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  enable = sqlc.arg(enable),
  updated_at = now()
WHERE team_id = app.current_team_id() AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteFetchProvider :exec
DELETE FROM fetch_providers WHERE team_id = app.current_team_id() AND id = sqlc.arg(id);
