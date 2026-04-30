-- name: CreateEmailOutbox :one
INSERT INTO email_outbox (id, provider_id, bot_id, from_address, to_addresses, subject, body_text, body_html, attachments, status)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(provider_id),
  sqlc.arg(bot_id),
  sqlc.arg(from_address),
  sqlc.arg(to_addresses),
  sqlc.arg(subject),
  sqlc.arg(body_text),
  sqlc.arg(body_html),
  sqlc.arg(attachments),
  sqlc.arg(status)
)
RETURNING *;

-- name: GetEmailOutboxByID :one
SELECT * FROM email_outbox WHERE id = sqlc.arg(id);

-- name: ListEmailOutboxByBot :many
SELECT * FROM email_outbox
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountEmailOutboxByBot :one
SELECT count(*) FROM email_outbox
WHERE bot_id = sqlc.arg(bot_id);

-- name: UpdateEmailOutboxSent :exec
UPDATE email_outbox
SET message_id = sqlc.arg(message_id), status = 'sent', sent_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: UpdateEmailOutboxFailed :exec
UPDATE email_outbox
SET status = 'failed', error = sqlc.arg(error)
WHERE id = sqlc.arg(id);
