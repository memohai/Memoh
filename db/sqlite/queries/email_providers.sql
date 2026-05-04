-- name: CreateEmailProvider :one
INSERT INTO email_providers (id, name, provider, config)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
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
  updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteEmailProvider :exec
DELETE FROM email_providers WHERE id = sqlc.arg(id);
