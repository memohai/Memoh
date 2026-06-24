-- name: CreateEmailProvider :one
INSERT INTO email_providers (id, user_id, name, provider, config)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(user_id),
  sqlc.arg(name),
  sqlc.arg(provider),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetEmailProviderByID :one
SELECT * FROM email_providers WHERE id = sqlc.arg(id);

-- name: GetEmailProviderByIDAndUser :one
SELECT * FROM email_providers
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id);

-- name: GetEmailProviderByNameAndUser :one
SELECT * FROM email_providers
WHERE user_id = sqlc.arg(user_id)
  AND name = sqlc.arg(name);

-- name: ListEmailProviders :many
SELECT * FROM email_providers
ORDER BY created_at DESC;

-- name: ListEmailProvidersByUser :many
SELECT * FROM email_providers
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC;

-- name: ListEmailProvidersByProvider :many
SELECT * FROM email_providers
WHERE provider = sqlc.arg(provider)
ORDER BY created_at DESC;

-- name: ListEmailProvidersByUserAndProvider :many
SELECT * FROM email_providers
WHERE user_id = sqlc.arg(user_id)
  AND provider = sqlc.arg(provider)
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

-- name: UpdateEmailProviderByIDAndUser :one
UPDATE email_providers
SET
  name = sqlc.arg(name),
  provider = sqlc.arg(provider),
  config = sqlc.arg(config),
  updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
RETURNING *;

-- name: DeleteEmailProvider :exec
DELETE FROM email_providers WHERE id = sqlc.arg(id);

-- name: DeleteEmailProviderByIDAndUser :exec
DELETE FROM email_providers
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id);
