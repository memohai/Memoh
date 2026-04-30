-- name: CreateSchedule :one
INSERT INTO schedule (id, name, description, pattern, max_calls, enabled, command, bot_id)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(name),
  sqlc.arg(description),
  sqlc.arg(pattern),
  sqlc.arg(max_calls),
  sqlc.arg(enabled),
  sqlc.arg(command),
  sqlc.arg(bot_id)
)
RETURNING id, name, description, pattern, max_calls, current_calls, created_at, updated_at, enabled, command, bot_id;

-- name: GetScheduleByID :one
SELECT id, name, description, pattern, max_calls, current_calls, created_at, updated_at, enabled, command, bot_id
FROM schedule
WHERE id = sqlc.arg(id);

-- name: ListSchedulesByBot :many
SELECT id, name, description, pattern, max_calls, current_calls, created_at, updated_at, enabled, command, bot_id
FROM schedule
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY created_at DESC;

-- name: ListEnabledSchedules :many
SELECT id, name, description, pattern, max_calls, current_calls, created_at, updated_at, enabled, command, bot_id
FROM schedule
WHERE enabled = true
ORDER BY created_at DESC;

-- name: UpdateSchedule :one
UPDATE schedule
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    pattern = sqlc.arg(pattern),
    max_calls = sqlc.arg(max_calls),
    enabled = sqlc.arg(enabled),
    command = sqlc.arg(command),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, name, description, pattern, max_calls, current_calls, created_at, updated_at, enabled, command, bot_id;

-- name: DeleteSchedule :exec
DELETE FROM schedule WHERE id = sqlc.arg(id);

-- name: IncrementScheduleCalls :one
UPDATE schedule
SET current_calls = current_calls + 1,
    enabled = CASE
      WHEN max_calls IS NOT NULL AND current_calls + 1 >= max_calls THEN false
      ELSE enabled
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, name, description, pattern, max_calls, current_calls, created_at, updated_at, enabled, command, bot_id;
