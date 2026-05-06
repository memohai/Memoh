-- name: CreateUser :one
INSERT INTO iam_users (is_active, metadata)
VALUES ($1, $2)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM iam_users
WHERE id = $1;

-- name: CreateAccount :one
UPDATE iam_users
SET username = sqlc.arg(username),
    email = sqlc.arg(email),
    display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    data_root = sqlc.arg(data_root),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: UpsertAccountByUsername :one
INSERT INTO iam_users (id, username, email, display_name, avatar_url, is_active, data_root, metadata)
VALUES (
  sqlc.arg(user_id),
  sqlc.arg(username),
  sqlc.arg(email),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(is_active),
  sqlc.arg(data_root),
  '{}'::jsonb
)
ON CONFLICT (username) DO UPDATE SET
  email = EXCLUDED.email,
  display_name = EXCLUDED.display_name,
  avatar_url = EXCLUDED.avatar_url,
  is_active = EXCLUDED.is_active,
  data_root = EXCLUDED.data_root,
  updated_at = now()
RETURNING *;

-- name: GetAccountByIdentity :one
SELECT u.*
FROM iam_users u
JOIN iam_identities i ON i.user_id = u.id
WHERE i.provider_type = 'password'
  AND (i.subject = lower(sqlc.arg(identity)::text) OR lower(COALESCE(i.email, '')) = lower(sqlc.arg(identity)::text))
LIMIT 1;

-- name: GetAccountByUserID :one
SELECT * FROM iam_users WHERE id = sqlc.arg(user_id);

-- name: CountAccounts :one
SELECT COUNT(DISTINCT u.id)::bigint AS count
FROM iam_users u
JOIN iam_identities i ON i.user_id = u.id
WHERE i.provider_type = 'password';

-- name: ListAccounts :many
SELECT DISTINCT u.*
FROM iam_users u
JOIN iam_identities i ON i.user_id = u.id
WHERE i.provider_type = 'password'
ORDER BY u.created_at DESC;

-- name: SearchAccounts :many
SELECT DISTINCT u.*
FROM iam_users u
LEFT JOIN iam_identities i ON i.user_id = u.id AND i.provider_type = 'password'
WHERE i.id IS NOT NULL
  AND (
    sqlc.arg(query)::text = ''
    OR u.username ILIKE '%' || sqlc.arg(query)::text || '%'
    OR COALESCE(u.display_name, '') ILIKE '%' || sqlc.arg(query)::text || '%'
    OR COALESCE(u.email, '') ILIKE '%' || sqlc.arg(query)::text || '%'
  )
ORDER BY u.last_login_at DESC NULLS LAST, u.created_at DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateAccountProfile :one
UPDATE iam_users
SET display_name = $2,
    avatar_url = $3,
    timezone = $4,
    is_active = $5,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAccountAdmin :one
UPDATE iam_users
SET display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: UpdateAccountPassword :one
WITH updated AS (
  UPDATE iam_identities
  SET credential_secret = sqlc.arg(password_hash),
      updated_at = now()
  WHERE user_id = sqlc.arg(user_id)
    AND provider_type = 'password'
  RETURNING user_id
)
SELECT u.*
FROM iam_users u
JOIN updated ON updated.user_id = u.id;

-- name: UpdateAccountLastLogin :one
UPDATE iam_users
SET last_login_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;
