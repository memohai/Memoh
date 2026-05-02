-- name: CreateBindCode :one
INSERT INTO iam_channel_identity_bind_codes (id, token, issued_by_user_id, channel_type, expires_at)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(token),
  sqlc.arg(issued_by_user_id),
  sqlc.arg(channel_type),
  sqlc.arg(expires_at)
)
RETURNING id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at;

-- name: GetBindCode :one
SELECT id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at
FROM iam_channel_identity_bind_codes
WHERE token = sqlc.arg(token);

-- name: GetBindCodeForUpdate :one
SELECT id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at
FROM iam_channel_identity_bind_codes
WHERE token = sqlc.arg(token);

-- name: MarkBindCodeUsed :one
UPDATE iam_channel_identity_bind_codes
SET used_at = CURRENT_TIMESTAMP, used_by_channel_identity_id = sqlc.arg(used_by_channel_identity_id)
WHERE id = sqlc.arg(id)
  AND used_at IS NULL
RETURNING id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at;
