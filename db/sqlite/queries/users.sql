-- name: CreateUser :one
INSERT INTO iam_users (id, is_active, metadata)
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
FROM iam_users
WHERE id = sqlc.arg(id);

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
  '{}'
)
ON CONFLICT (username) DO UPDATE SET
  email = EXCLUDED.email,
  display_name = EXCLUDED.display_name,
  avatar_url = EXCLUDED.avatar_url,
  is_active = EXCLUDED.is_active,
  data_root = EXCLUDED.data_root,
  updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: CreateAccount :one
UPDATE iam_users
SET username = sqlc.arg(username),
    email = sqlc.arg(email),
    display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    data_root = sqlc.arg(data_root),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: GetAccountByIdentity :one
SELECT u.*
FROM iam_users u
JOIN iam_identities i ON i.user_id = u.id
WHERE i.provider_type = 'password'
  AND (i.subject = lower(sqlc.arg(identity)) OR lower(COALESCE(i.email, '')) = lower(sqlc.arg(identity)))
LIMIT 1;

-- name: GetAccountByUserID :one
SELECT * FROM iam_users WHERE id = sqlc.arg(user_id);

-- name: CountAccounts :one
SELECT COUNT(DISTINCT u.id) AS count
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
    sqlc.arg(query) = ''
    OR lower(u.username) LIKE '%' || lower(sqlc.arg(query)) || '%'
    OR lower(COALESCE(u.display_name, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
    OR lower(COALESCE(u.email, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
  )
ORDER BY u.last_login_at DESC, u.created_at DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateAccountProfile :one
UPDATE iam_users
SET display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    timezone = sqlc.arg(timezone),
    is_active = sqlc.arg(is_active),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateAccountAdmin :one
UPDATE iam_users
SET display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    is_active = sqlc.arg(is_active),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(user_id)
RETURNING *;

-- name: UpdateAccountPassword :one
UPDATE iam_identities
SET credential_secret = sqlc.arg(password_hash),
    updated_at = CURRENT_TIMESTAMP
WHERE user_id = sqlc.arg(user_id)
  AND provider_type = 'password'
RETURNING (
  SELECT id FROM iam_users WHERE id = iam_identities.user_id
) AS id,
(
  SELECT username FROM iam_users WHERE id = iam_identities.user_id
) AS username,
(
  SELECT email FROM iam_users WHERE id = iam_identities.user_id
) AS email,
(
  SELECT display_name FROM iam_users WHERE id = iam_identities.user_id
) AS display_name,
(
  SELECT avatar_url FROM iam_users WHERE id = iam_identities.user_id
) AS avatar_url,
(
  SELECT timezone FROM iam_users WHERE id = iam_identities.user_id
) AS timezone,
(
  SELECT data_root FROM iam_users WHERE id = iam_identities.user_id
) AS data_root,
(
  SELECT last_login_at FROM iam_users WHERE id = iam_identities.user_id
) AS last_login_at,
(
  SELECT is_active FROM iam_users WHERE id = iam_identities.user_id
) AS is_active,
(
  SELECT metadata FROM iam_users WHERE id = iam_identities.user_id
) AS metadata,
(
  SELECT created_at FROM iam_users WHERE id = iam_identities.user_id
) AS created_at,
(
  SELECT updated_at FROM iam_users WHERE id = iam_identities.user_id
) AS updated_at;

-- name: UpdateAccountLastLogin :one
UPDATE iam_users
SET last_login_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;
