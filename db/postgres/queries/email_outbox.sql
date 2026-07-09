-- name: CreateEmailOutbox :one
INSERT INTO email_outbox (provider_id, bot_id, from_address, to_addresses, subject, body_text, body_html, attachments, status)
SELECT
  ep.id,
  b.id,
  sqlc.arg(from_address),
  sqlc.arg(to_addresses),
  sqlc.arg(subject),
  sqlc.arg(body_text),
  sqlc.arg(body_html),
  sqlc.arg(attachments),
  sqlc.arg(status)
FROM email_providers ep
JOIN bots b ON b.id = sqlc.arg(bot_id)
WHERE ep.id = sqlc.arg(provider_id)
  AND ep.team_id = sqlc.arg(team_id)
  AND b.team_id = sqlc.arg(team_id)
RETURNING *;

-- name: GetEmailOutboxByID :one
SELECT eo.*
FROM email_outbox eo
JOIN bots b ON b.id = eo.bot_id
JOIN email_providers ep ON ep.id = eo.provider_id
WHERE eo.id = sqlc.arg(id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id);

-- name: ListEmailOutboxByBot :many
SELECT eo.*
FROM email_outbox eo
JOIN bots b ON b.id = eo.bot_id
JOIN email_providers ep ON ep.id = eo.provider_id
WHERE eo.bot_id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id)
ORDER BY eo.created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountEmailOutboxByBot :one
SELECT count(*)
FROM email_outbox eo
JOIN bots b ON b.id = eo.bot_id
JOIN email_providers ep ON ep.id = eo.provider_id
WHERE eo.bot_id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id);

-- name: UpdateEmailOutboxSent :exec
UPDATE email_outbox AS eo
SET message_id = sqlc.arg(message_id), status = 'sent', sent_at = now()
FROM bots b, email_providers ep
WHERE eo.id = sqlc.arg(id)
  AND b.id = eo.bot_id
  AND ep.id = eo.provider_id
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id);

-- name: UpdateEmailOutboxFailed :exec
UPDATE email_outbox AS eo
SET status = 'failed', error = sqlc.arg(error)
FROM bots b, email_providers ep
WHERE eo.id = sqlc.arg(id)
  AND b.id = eo.bot_id
  AND ep.id = eo.provider_id
  AND b.team_id = sqlc.arg(team_id)
  AND ep.team_id = sqlc.arg(team_id);
