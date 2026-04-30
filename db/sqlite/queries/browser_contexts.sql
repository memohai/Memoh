-- name: CreateBrowserContext :one
INSERT INTO browser_contexts (id, name, config)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(name),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetBrowserContextByID :one
SELECT * FROM browser_contexts WHERE id = sqlc.arg(id);

-- name: ListBrowserContexts :many
SELECT * FROM browser_contexts ORDER BY created_at DESC;

-- name: UpdateBrowserContext :one
UPDATE browser_contexts
SET name = sqlc.arg(name),
    config = sqlc.arg(config),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteBrowserContext :exec
DELETE FROM browser_contexts WHERE id = sqlc.arg(id);
