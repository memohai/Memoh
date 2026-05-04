-- name: CreateScheduleLog :one
INSERT INTO schedule_logs (id, schedule_id, bot_id, session_id, started_at)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(schedule_id),
  sqlc.arg(bot_id),
  sqlc.narg(session_id),
  CURRENT_TIMESTAMP
)
RETURNING id, schedule_id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at;

-- name: CompleteScheduleLog :one
UPDATE schedule_logs
SET status = sqlc.arg(status),
    result_text = sqlc.arg(result_text),
    error_message = sqlc.arg(error_message),
    usage = sqlc.arg(usage),
    model_id = sqlc.arg(model_id),
    completed_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, schedule_id, bot_id, session_id, status, result_text, error_message, usage, model_id, started_at, completed_at;

-- name: ListScheduleLogsByBot :many
SELECT id, schedule_id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at
FROM schedule_logs
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY started_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountScheduleLogsByBot :one
SELECT count(*) FROM schedule_logs WHERE bot_id = sqlc.arg(bot_id);

-- name: ListScheduleLogsBySchedule :many
SELECT id, schedule_id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at
FROM schedule_logs
WHERE schedule_id = sqlc.arg(schedule_id)
ORDER BY started_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountScheduleLogsBySchedule :one
SELECT count(*) FROM schedule_logs WHERE schedule_id = sqlc.arg(schedule_id);

-- name: DeleteScheduleLogsByBot :exec
DELETE FROM schedule_logs WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteScheduleLogsBySchedule :exec
DELETE FROM schedule_logs WHERE schedule_id = sqlc.arg(schedule_id);
