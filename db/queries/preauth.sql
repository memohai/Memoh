-- name: CreateBotPreauthKey :one
INSERT INTO bot_preauth_keys (bot_id, token, issued_by_user_id, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, bot_id, token, issued_by_user_id, expires_at, used_at, created_at;

-- name: GetBotPreauthKey :one
SELECT id, bot_id, token, issued_by_user_id, expires_at, used_at, created_at
FROM bot_preauth_keys
WHERE token = $1
  AND used_at IS NULL
  AND (expires_at IS NULL OR expires_at > now())
LIMIT 1;

-- name: MarkBotPreauthKeyUsed :one
UPDATE bot_preauth_keys
SET used_at = now()
WHERE id = $1
RETURNING id, bot_id, token, issued_by_user_id, expires_at, used_at, created_at;
