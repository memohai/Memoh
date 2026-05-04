-- name: CreateUser :one
INSERT INTO users (id, is_active, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(is_active),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = sqlc.arg(id);

-- name: UpsertAccountByUsername :one
INSERT INTO users (id, username, email, password_hash, role, display_name, avatar_url, is_active, data_root, metadata)
VALUES (
  sqlc.arg(user_id),
  sqlc.arg(username),
  sqlc.arg(email),
  sqlc.arg(password_hash),
  sqlc.arg(role),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(is_active),
  sqlc.arg(data_root),
  '{}'
)
ON CONFLICT (username) DO UPDATE SET
  email = EXCLUDED.email,
  password_hash = EXCLUDED.password_hash,
  role = EXCLUDED.role,
  display_name = EXCLUDED.display_name,
  avatar_url = EXCLUDED.avatar_url,
  is_active = EXCLUDED.is_active,
  data_root = EXCLUDED.data_root,
  updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: CreateAccount :one
UPDATE users
SET username = sqlc.arg(username),
    email = sqlc.arg(email),
    password_hash = sqlc.arg(password_hash),
    role = sqlc.arg(role),
    display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    data_root = sqlc.arg(data_root),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: GetAccountByIdentity :one
SELECT * FROM users WHERE username = sqlc.arg(identity) OR email = sqlc.arg(identity);

-- name: GetAccountByUserID :one
SELECT * FROM users WHERE id = sqlc.arg(user_id);

-- name: CountAccounts :one
SELECT COUNT(*) AS count
FROM users
WHERE username IS NOT NULL
  AND password_hash IS NOT NULL;

-- name: ListAccounts :many
SELECT * FROM users
WHERE username IS NOT NULL
ORDER BY created_at DESC;

-- name: SearchAccounts :many
SELECT *
FROM users
WHERE username IS NOT NULL
  AND (
    sqlc.arg(query) = ''
    OR lower(username) LIKE '%' || lower(sqlc.arg(query)) || '%'
    OR lower(COALESCE(display_name, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
    OR lower(COALESCE(email, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
  )
ORDER BY last_login_at DESC, created_at DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateAccountProfile :one
UPDATE users
SET display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    timezone = sqlc.arg(timezone),
    is_active = sqlc.arg(is_active),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateAccountAdmin :one
UPDATE users
SET role = sqlc.arg(role),
    display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: UpdateAccountPassword :one
UPDATE users
SET password_hash = sqlc.arg(password_hash),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateAccountLastLogin :one
UPDATE users
SET last_login_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;
