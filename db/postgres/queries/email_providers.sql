-- name: CreateEmailProvider :one
INSERT INTO email_providers (name, provider, config)
VALUES (
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetEmailProviderByID :one
SELECT * FROM email_providers WHERE id = sqlc.arg(id);

-- name: GetEmailProviderByName :one
SELECT * FROM email_providers WHERE name = sqlc.arg(name);

-- name: ListEmailProviders :many
SELECT * FROM email_providers
ORDER BY created_at DESC;

-- name: ListEmailProvidersByProvider :many
SELECT * FROM email_providers
WHERE provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: UpdateEmailProvider :one
UPDATE email_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteEmailProvider :exec
DELETE FROM email_providers WHERE id = sqlc.arg(id);
