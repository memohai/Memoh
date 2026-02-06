-- name: CreateContact :one
INSERT INTO contacts (bot_id, user_id, display_name, alias, tags, status, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, bot_id, user_id, display_name, alias, tags, status, metadata, created_at, updated_at;

-- name: GetContactByID :one
SELECT id, bot_id, user_id, display_name, alias, tags, status, metadata, created_at, updated_at
FROM contacts
WHERE id = $1
LIMIT 1;

-- name: GetContactByUserID :one
SELECT id, bot_id, user_id, display_name, alias, tags, status, metadata, created_at, updated_at
FROM contacts
WHERE bot_id = $1 AND user_id = $2
LIMIT 1;

-- name: ListContactsByBot :many
SELECT id, bot_id, user_id, display_name, alias, tags, status, metadata, created_at, updated_at
FROM contacts
WHERE bot_id = $1
ORDER BY created_at DESC;

-- name: SearchContacts :many
SELECT id, bot_id, user_id, display_name, alias, tags, status, metadata, created_at, updated_at
FROM contacts
WHERE bot_id = $1
  AND (
    display_name ILIKE sqlc.arg(query)
    OR alias ILIKE sqlc.arg(query)
    OR EXISTS (
      SELECT 1 FROM unnest(tags) AS tag WHERE tag ILIKE sqlc.arg(query)
    )
  )
ORDER BY created_at DESC;

-- name: UpdateContact :one
UPDATE contacts
SET display_name = COALESCE(sqlc.narg(display_name), display_name),
    alias = COALESCE(sqlc.narg(alias), alias),
    tags = COALESCE(sqlc.narg(tags), tags),
    status = COALESCE(NULLIF(sqlc.arg(status)::text, ''), status),
    metadata = COALESCE(sqlc.narg(metadata), metadata),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, bot_id, user_id, display_name, alias, tags, status, metadata, created_at, updated_at;

-- name: UpdateContactUser :one
UPDATE contacts
SET user_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, bot_id, user_id, display_name, alias, tags, status, metadata, created_at, updated_at;

-- name: UpsertContactChannel :one
INSERT INTO contact_channels (bot_id, contact_id, platform, external_id, metadata)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (bot_id, platform, external_id)
DO UPDATE SET
  contact_id = EXCLUDED.contact_id,
  metadata = EXCLUDED.metadata,
  updated_at = now()
RETURNING id, bot_id, contact_id, platform, external_id, metadata, created_at, updated_at;

-- name: GetContactChannelByIdentity :one
SELECT id, bot_id, contact_id, platform, external_id, metadata, created_at, updated_at
FROM contact_channels
WHERE bot_id = $1 AND platform = $2 AND external_id = $3
LIMIT 1;

-- name: ListContactChannelsByContact :many
SELECT id, bot_id, contact_id, platform, external_id, metadata, created_at, updated_at
FROM contact_channels
WHERE contact_id = $1
ORDER BY created_at DESC;

