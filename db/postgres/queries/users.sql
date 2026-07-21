-- name: CreateUser :one
WITH created_user AS (
  INSERT INTO users (is_active, metadata)
  VALUES (sqlc.arg(is_active), sqlc.arg(metadata))
  RETURNING users.*
), created_membership AS (
  INSERT INTO team_members (team_id, user_id, is_active)
  SELECT public.memoh_current_team_id(), id, sqlc.arg(is_active)
  FROM created_user
  RETURNING team_members.*
)
SELECT
  changed_user.id, changed_user.username, changed_user.email,
  changed_user.password_hash, changed_membership.role,
  changed_user.display_name, changed_user.avatar_url, changed_user.timezone,
  changed_membership.data_root, changed_user.last_login_at,
  (changed_user.is_active AND changed_membership.is_active) AS is_active,
  changed_user.metadata, changed_user.created_at, changed_user.updated_at,
  changed_membership.team_id,
  changed_user.is_active AS principal_is_active,
  changed_membership.is_active AS membership_is_active,
  changed_membership.created_at AS joined_at,
  changed_membership.updated_at AS membership_updated_at,
  changed_membership.title_model_id
FROM created_user changed_user
JOIN created_membership changed_membership
  ON changed_membership.user_id = changed_user.id;

-- name: GetUserByID :one
SELECT users.*
FROM users
JOIN team_members membership
  ON membership.user_id = users.id
 AND membership.team_id = public.memoh_current_team_id()
WHERE users.id = sqlc.arg(user_id);

-- name: CreateAccount :one
WITH updated_user AS (
  UPDATE users
  SET username = sqlc.arg(username),
      email = sqlc.arg(email),
      password_hash = sqlc.arg(password_hash),
      display_name = sqlc.arg(display_name),
      avatar_url = sqlc.arg(avatar_url),
      updated_at = now()
  WHERE users.id = sqlc.arg(user_id)
    AND EXISTS (
      SELECT 1 FROM team_members membership
      WHERE membership.team_id = public.memoh_current_team_id()
        AND membership.user_id = users.id
    )
  RETURNING users.*
), updated_membership AS (
  UPDATE team_members membership
  SET role = sqlc.arg(role)::user_role,
      is_active = sqlc.arg(is_active),
      data_root = sqlc.arg(data_root),
      updated_at = now()
  FROM updated_user
  WHERE membership.team_id = public.memoh_current_team_id()
    AND membership.user_id = updated_user.id
  RETURNING membership.*
)
SELECT
  changed_user.id, changed_user.username, changed_user.email,
  changed_user.password_hash, changed_membership.role,
  changed_user.display_name, changed_user.avatar_url, changed_user.timezone,
  changed_membership.data_root, changed_user.last_login_at,
  (changed_user.is_active AND changed_membership.is_active) AS is_active,
  changed_user.metadata, changed_user.created_at, changed_user.updated_at,
  changed_membership.team_id,
  changed_user.is_active AS principal_is_active,
  changed_membership.is_active AS membership_is_active,
  changed_membership.created_at AS joined_at,
  changed_membership.updated_at AS membership_updated_at,
  changed_membership.title_model_id
FROM updated_user changed_user
JOIN updated_membership changed_membership
  ON changed_membership.user_id = changed_user.id;

-- name: UpsertAccountByUsername :one
WITH upserted_user AS (
  INSERT INTO users (
    id, username, email, password_hash, display_name, avatar_url,
    is_active, metadata
  )
  VALUES (
    sqlc.arg(user_id),
    sqlc.arg(username),
    sqlc.arg(email),
    sqlc.arg(password_hash),
    sqlc.arg(display_name),
    sqlc.arg(avatar_url),
    sqlc.arg(is_active),
    '{}'::jsonb
  )
  ON CONFLICT (username) DO NOTHING
  RETURNING users.*
), selected_user AS (
  SELECT * FROM upserted_user
  UNION ALL
  SELECT users.*
  FROM users
  WHERE users.username = sqlc.arg(username)
    AND NOT EXISTS (SELECT 1 FROM upserted_user)
), upserted_membership AS (
  INSERT INTO team_members (
    team_id, user_id, role, is_active, data_root
  )
  SELECT
    public.memoh_current_team_id(),
    id,
    sqlc.arg(role)::user_role,
    sqlc.arg(is_active),
    sqlc.arg(data_root)
  FROM selected_user
  ON CONFLICT (team_id, user_id) DO UPDATE SET
    role = EXCLUDED.role,
    is_active = EXCLUDED.is_active,
    data_root = EXCLUDED.data_root,
    updated_at = now()
  RETURNING team_members.*
)
SELECT
  changed_user.id, changed_user.username, changed_user.email,
  changed_user.password_hash, changed_membership.role,
  changed_user.display_name, changed_user.avatar_url, changed_user.timezone,
  changed_membership.data_root, changed_user.last_login_at,
  (changed_user.is_active AND changed_membership.is_active) AS is_active,
  changed_user.metadata, changed_user.created_at, changed_user.updated_at,
  changed_membership.team_id,
  changed_user.is_active AS principal_is_active,
  changed_membership.is_active AS membership_is_active,
  changed_membership.created_at AS joined_at,
  changed_membership.updated_at AS membership_updated_at,
  changed_membership.title_model_id
FROM selected_user changed_user
JOIN upserted_membership changed_membership
  ON changed_membership.user_id = changed_user.id;

-- name: GetAccountByIdentity :one
SELECT *
FROM team_accounts
WHERE username = sqlc.arg(identity) OR email = sqlc.arg(identity);

-- name: GetAccountByUserID :one
SELECT * FROM team_accounts WHERE id = sqlc.arg(user_id);

-- name: CountAccounts :one
SELECT COUNT(*)::bigint AS count
FROM team_accounts
WHERE username IS NOT NULL
  AND password_hash IS NOT NULL;

-- name: ListAccounts :many
SELECT * FROM team_accounts
WHERE username IS NOT NULL
ORDER BY created_at DESC;

-- name: SearchAccounts :many
SELECT *
FROM team_accounts
WHERE username IS NOT NULL
  AND (
    sqlc.arg(query)::text = ''
    OR username ILIKE '%' || sqlc.arg(query)::text || '%'
    OR COALESCE(display_name, '') ILIKE '%' || sqlc.arg(query)::text || '%'
    OR COALESCE(email, '') ILIKE '%' || sqlc.arg(query)::text || '%'
  )
ORDER BY last_login_at DESC NULLS LAST, created_at DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateAccountProfile :one
WITH updated_user AS (
  UPDATE users
  SET display_name = sqlc.arg(display_name),
      avatar_url = sqlc.arg(avatar_url),
      timezone = sqlc.arg(timezone),
      metadata = sqlc.arg(metadata),
      updated_at = now()
  WHERE users.id = sqlc.arg(user_id)
    AND EXISTS (
      SELECT 1 FROM team_members membership
      WHERE membership.team_id = public.memoh_current_team_id()
        AND membership.user_id = users.id
    )
  RETURNING users.*
), updated_membership AS (
  UPDATE team_members membership
  SET title_model_id = sqlc.narg(title_model_id)::uuid,
      updated_at = CASE
        WHEN membership.title_model_id IS DISTINCT FROM sqlc.narg(title_model_id)::uuid THEN now()
        ELSE membership.updated_at
      END
  FROM updated_user
  WHERE membership.team_id = public.memoh_current_team_id()
    AND membership.user_id = updated_user.id
  RETURNING membership.*
)
SELECT
  changed_user.id, changed_user.username, changed_user.email,
  changed_user.password_hash, changed_membership.role,
  changed_user.display_name, changed_user.avatar_url, changed_user.timezone,
  changed_membership.data_root, changed_user.last_login_at,
  (changed_user.is_active AND changed_membership.is_active) AS is_active,
  changed_user.metadata, changed_user.created_at, changed_user.updated_at,
  changed_membership.team_id,
  changed_user.is_active AS principal_is_active,
  changed_membership.is_active AS membership_is_active,
  changed_membership.created_at AS joined_at,
  changed_membership.updated_at AS membership_updated_at,
  changed_membership.title_model_id
FROM updated_user changed_user
JOIN updated_membership changed_membership
  ON changed_membership.user_id = changed_user.id;

-- name: UpdateAccountAdmin :one
WITH updated_membership AS (
  UPDATE team_members membership
  SET role = sqlc.arg(role)::user_role,
      is_active = COALESCE(sqlc.narg(is_active)::boolean, membership.is_active),
      updated_at = now()
  WHERE membership.team_id = public.memoh_current_team_id()
    AND membership.user_id = sqlc.arg(user_id)
  RETURNING membership.*
)
SELECT
  changed_user.id, changed_user.username, changed_user.email,
  changed_user.password_hash, changed_membership.role,
  changed_user.display_name, changed_user.avatar_url, changed_user.timezone,
  changed_membership.data_root, changed_user.last_login_at,
  (changed_user.is_active AND changed_membership.is_active) AS is_active,
  changed_user.metadata, changed_user.created_at, changed_user.updated_at,
  changed_membership.team_id,
  changed_user.is_active AS principal_is_active,
  changed_membership.is_active AS membership_is_active,
  changed_membership.created_at AS joined_at,
  changed_membership.updated_at AS membership_updated_at,
  changed_membership.title_model_id
FROM updated_membership changed_membership
JOIN users changed_user ON changed_user.id = changed_membership.user_id;

-- name: UpdateAccountPassword :one
UPDATE users
SET password_hash = sqlc.arg(password_hash),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
  AND EXISTS (
    SELECT 1 FROM team_members membership
    WHERE membership.team_id = public.memoh_current_team_id()
      AND membership.user_id = users.id
  )
RETURNING id;

-- name: RemoveMember :one
UPDATE team_members
SET is_active = FALSE,
    updated_at = now()
WHERE team_id = public.memoh_current_team_id()
  AND user_id = sqlc.arg(user_id)
RETURNING user_id;

-- name: UpdateAccountLastLogin :one
UPDATE users
SET last_login_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
  AND EXISTS (
    SELECT 1 FROM team_members membership
    WHERE membership.team_id = public.memoh_current_team_id()
      AND membership.user_id = users.id
  )
RETURNING id;
