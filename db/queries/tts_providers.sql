-- name: CreateTtsProvider :one
INSERT INTO tts_providers (name, provider, config)
VALUES (
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetTtsProviderByID :one
SELECT * FROM tts_providers WHERE id = sqlc.arg(id);

-- name: GetTtsProviderByName :one
SELECT * FROM tts_providers WHERE name = sqlc.arg(name);

-- name: ListTtsProviders :many
SELECT * FROM tts_providers
ORDER BY created_at DESC;

-- name: ListTtsProvidersByProvider :many
SELECT * FROM tts_providers
WHERE provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateTtsProvider :one
UPDATE tts_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteTtsProvider :exec
DELETE FROM tts_providers WHERE id = sqlc.arg(id);
