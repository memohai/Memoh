-- name: CreateBrowserContext :one
INSERT INTO browser_contexts (name, config)
VALUES (sqlc.arg(name), sqlc.arg(config))
RETURNING *;

-- name: GetBrowserContextByID :one
SELECT * FROM browser_contexts WHERE id = $1;

-- name: ListBrowserContexts :many
SELECT * FROM browser_contexts ORDER BY created_at DESC;

-- name: UpdateBrowserContext :one
UPDATE browser_contexts
SET name = sqlc.arg(name),
    config = sqlc.arg(config),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteBrowserContext :exec
DELETE FROM browser_contexts WHERE id = $1;
