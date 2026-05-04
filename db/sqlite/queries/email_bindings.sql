-- name: CreateBotEmailBinding :one
INSERT INTO bot_email_bindings (id, bot_id, email_provider_id, email_address, can_read, can_write, can_delete, config)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(email_provider_id),
  sqlc.arg(email_address),
  sqlc.arg(can_read),
  sqlc.arg(can_write),
  sqlc.arg(can_delete),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetBotEmailBindingByID :one
SELECT * FROM bot_email_bindings WHERE id = sqlc.arg(id);

-- name: GetBotEmailBindingByBotAndProvider :one
SELECT * FROM bot_email_bindings
WHERE bot_id = sqlc.arg(bot_id) AND email_provider_id = sqlc.arg(email_provider_id);

-- name: ListBotEmailBindings :many
SELECT * FROM bot_email_bindings
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY created_at DESC;

-- name: ListBotEmailBindingsByProvider :many
SELECT * FROM bot_email_bindings
WHERE email_provider_id = sqlc.arg(email_provider_id)
ORDER BY created_at DESC;

-- name: ListReadableBindingsByProvider :many
SELECT * FROM bot_email_bindings
WHERE email_provider_id = sqlc.arg(email_provider_id) AND can_read = TRUE
ORDER BY created_at DESC;

-- name: UpdateBotEmailBinding :one
UPDATE bot_email_bindings
SET
  email_address = sqlc.arg(email_address),
  can_read = sqlc.arg(can_read),
  can_write = sqlc.arg(can_write),
  can_delete = sqlc.arg(can_delete),
  config = sqlc.arg(config),
  updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteBotEmailBinding :exec
DELETE FROM bot_email_bindings WHERE id = sqlc.arg(id);
