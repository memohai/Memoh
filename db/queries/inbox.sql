-- name: CreateInboxItem :one
INSERT INTO bot_inbox (bot_id, source, header, content, action)
VALUES (sqlc.arg(bot_id), sqlc.arg(source), sqlc.arg(header), sqlc.arg(content), sqlc.arg(action))
RETURNING *;

-- name: GetInboxItemByID :one
SELECT * FROM bot_inbox
WHERE id = sqlc.arg(id)
  AND bot_id = sqlc.arg(bot_id);

-- name: ListInboxItems :many
SELECT * FROM bot_inbox
WHERE bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(is_read)::boolean IS NULL OR is_read = sqlc.narg(is_read)::boolean)
  AND (sqlc.narg(source)::text IS NULL OR source = sqlc.narg(source)::text)
ORDER BY created_at DESC
LIMIT sqlc.arg(max_count)
OFFSET sqlc.arg(item_offset);

-- name: ListUnreadInboxItems :many
SELECT * FROM bot_inbox
WHERE bot_id = sqlc.arg(bot_id)
  AND is_read = FALSE
ORDER BY created_at ASC
LIMIT sqlc.arg(max_count);

-- name: MarkInboxItemsRead :exec
UPDATE bot_inbox
SET is_read = TRUE,
    read_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND id = ANY(sqlc.arg(ids)::uuid[])
  AND is_read = FALSE;

-- name: SearchInboxItems :many
SELECT * FROM bot_inbox
WHERE bot_id = sqlc.arg(bot_id)
  AND content ILIKE '%' || sqlc.arg(query) || '%'
  AND (sqlc.narg(start_time)::timestamptz IS NULL OR created_at >= sqlc.narg(start_time)::timestamptz)
  AND (sqlc.narg(end_time)::timestamptz IS NULL OR created_at <= sqlc.narg(end_time)::timestamptz)
  AND (sqlc.narg(include_read)::boolean IS NULL OR sqlc.narg(include_read)::boolean = TRUE OR is_read = FALSE)
ORDER BY created_at DESC
LIMIT sqlc.arg(max_count);

-- name: CountUnreadInboxItems :one
SELECT count(*) FROM bot_inbox
WHERE bot_id = sqlc.arg(bot_id)
  AND is_read = FALSE;

-- name: CountInboxItems :one
SELECT count(*) FROM bot_inbox
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteInboxItem :exec
DELETE FROM bot_inbox
WHERE id = sqlc.arg(id)
  AND bot_id = sqlc.arg(bot_id);

-- name: DeleteInboxItemsByBot :exec
DELETE FROM bot_inbox
WHERE bot_id = sqlc.arg(bot_id);
