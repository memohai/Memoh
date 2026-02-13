-- name: CreateUser :one
INSERT INTO users (is_active, metadata)
VALUES ($1, $2)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: CreateAccount :one
UPDATE users
SET username = sqlc.arg(username),
    email = sqlc.arg(email),
    password_hash = sqlc.arg(password_hash),
    role = sqlc.arg(role)::user_role,
    display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    data_root = sqlc.arg(data_root),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: UpsertAccountByUsername :one
INSERT INTO users (id, username, email, password_hash, role, display_name, avatar_url, is_active, data_root, metadata)
VALUES (
  sqlc.arg(user_id),
  sqlc.arg(username),
  sqlc.arg(email),
  sqlc.arg(password_hash),
  sqlc.arg(role)::user_role,
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(is_active),
  sqlc.arg(data_root),
  '{}'::jsonb
)
ON CONFLICT (username) DO UPDATE SET
  email = EXCLUDED.email,
  password_hash = EXCLUDED.password_hash,
  role = EXCLUDED.role,
  display_name = EXCLUDED.display_name,
  avatar_url = EXCLUDED.avatar_url,
  is_active = EXCLUDED.is_active,
  data_root = EXCLUDED.data_root,
  updated_at = now()
RETURNING *;

-- name: GetAccountByIdentity :one
SELECT * FROM users WHERE username = sqlc.arg(identity) OR email = sqlc.arg(identity);

-- name: GetAccountByUserID :one
SELECT * FROM users WHERE id = sqlc.arg(user_id);

-- name: CountAccounts :one
SELECT COUNT(*)::bigint AS count
FROM users
WHERE username IS NOT NULL
  AND password_hash IS NOT NULL;

-- name: ListAccounts :many
SELECT * FROM users
WHERE username IS NOT NULL
ORDER BY created_at DESC;

-- name: UpdateAccountProfile :one
UPDATE users
SET display_name = $2,
    avatar_url = $3,
    is_active = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAccountAdmin :one
UPDATE users
SET role = sqlc.arg(role)::user_role,
    display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: UpdateAccountPassword :one
UPDATE users
SET password_hash = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateAccountLastLogin :one
UPDATE users
SET last_login_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;
