-- name: CreateBindCode :one
INSERT INTO channel_identity_bind_codes (token, issued_by_user_id, channel_type, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at;

-- name: GetBindCode :one
SELECT id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at
FROM channel_identity_bind_codes
WHERE token = $1;

-- name: GetBindCodeForUpdate :one
SELECT id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at
FROM channel_identity_bind_codes
WHERE token = $1
FOR UPDATE;

-- name: MarkBindCodeUsed :one
UPDATE channel_identity_bind_codes
SET used_at = now(), used_by_channel_identity_id = $2
WHERE id = $1
  AND used_at IS NULL
RETURNING id, token, issued_by_user_id, channel_type, expires_at, used_at, used_by_channel_identity_id, created_at;
