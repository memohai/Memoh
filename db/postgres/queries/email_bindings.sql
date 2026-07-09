-- name: CreateBotEmailBinding :one
INSERT INTO bot_email_bindings (bot_id, email_provider_id, email_address, can_read, can_write, can_delete, config)
SELECT
  b.id,
  ep.id,
  sqlc.arg(email_address),
  sqlc.arg(can_read),
  sqlc.arg(can_write),
  sqlc.arg(can_delete),
  sqlc.arg(config)
FROM bots b
JOIN email_providers ep ON ep.id = sqlc.arg(email_provider_id)
WHERE b.id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id)
RETURNING *;

-- name: GetBotEmailBindingByID :one
SELECT beb.*
FROM bot_email_bindings beb
JOIN bots b ON b.id = beb.bot_id
JOIN email_providers ep ON ep.id = beb.email_provider_id
WHERE beb.id = sqlc.arg(id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id);

-- name: GetBotEmailBindingByBotAndProvider :one
SELECT beb.*
FROM bot_email_bindings beb
JOIN bots b ON b.id = beb.bot_id
JOIN email_providers ep ON ep.id = beb.email_provider_id
WHERE beb.bot_id = sqlc.arg(bot_id)
  AND beb.email_provider_id = sqlc.arg(email_provider_id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id);

-- name: ListBotEmailBindings :many
SELECT beb.*
FROM bot_email_bindings beb
JOIN bots b ON b.id = beb.bot_id
JOIN email_providers ep ON ep.id = beb.email_provider_id
WHERE beb.bot_id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id)
ORDER BY beb.created_at DESC;

-- name: ListBotEmailBindingsByProvider :many
SELECT beb.*
FROM bot_email_bindings beb
JOIN bots b ON b.id = beb.bot_id
JOIN email_providers ep ON ep.id = beb.email_provider_id
WHERE beb.email_provider_id = sqlc.arg(email_provider_id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id)
ORDER BY beb.created_at DESC;

-- name: ListReadableBindingsByProvider :many
SELECT beb.*
FROM bot_email_bindings beb
JOIN bots b ON b.id = beb.bot_id
JOIN email_providers ep ON ep.id = beb.email_provider_id
WHERE beb.email_provider_id = sqlc.arg(email_provider_id)
  AND beb.can_read = TRUE
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id)
ORDER BY beb.created_at DESC;

-- name: UpdateBotEmailBinding :one
UPDATE bot_email_bindings AS beb
SET
  email_address = sqlc.arg(email_address),
  can_read = sqlc.arg(can_read),
  can_write = sqlc.arg(can_write),
  can_delete = sqlc.arg(can_delete),
  config = sqlc.arg(config),
  updated_at = now()
FROM bots b, email_providers ep
WHERE beb.id = sqlc.arg(id)
  AND b.id = beb.bot_id
  AND ep.id = beb.email_provider_id
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id)
RETURNING beb.*;

-- name: DeleteBotEmailBinding :exec
DELETE FROM bot_email_bindings AS beb
USING bots b, email_providers ep
WHERE beb.id = sqlc.arg(id)
  AND b.id = beb.bot_id
  AND ep.id = beb.email_provider_id
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id);
