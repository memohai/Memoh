-- name: CreateScheduleLog :one
INSERT INTO schedule_logs (team_id, schedule_id, bot_id, session_id, started_at)
VALUES (sqlc.arg(team_id)::uuid, sqlc.arg(schedule_id)::uuid, sqlc.arg(bot_id)::uuid, sqlc.narg(session_id)::uuid, now())
RETURNING id, team_id, schedule_id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at;

-- name: CompleteScheduleLog :one
UPDATE schedule_logs
SET status = sqlc.arg(status),
    result_text = sqlc.arg(result_text),
    error_message = sqlc.arg(error_message),
    usage = sqlc.arg(usage),
    model_id = sqlc.arg(model_id),
    completed_at = now()
WHERE id = sqlc.arg(id)::uuid
  AND team_id = sqlc.arg(team_id)::uuid
RETURNING *;

-- name: ListScheduleLogsByBot :many
SELECT id, team_id, schedule_id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at
FROM schedule_logs
WHERE team_id = sqlc.arg(team_id)::uuid
  AND bot_id = sqlc.arg(bot_id)::uuid
ORDER BY started_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountScheduleLogsByBot :one
SELECT count(*) FROM schedule_logs
WHERE team_id = sqlc.arg(team_id)::uuid
  AND bot_id = sqlc.arg(bot_id)::uuid;

-- name: ListScheduleLogsBySchedule :many
SELECT id, team_id, schedule_id, bot_id, session_id, status, result_text, error_message, usage, started_at, completed_at
FROM schedule_logs
WHERE team_id = sqlc.arg(team_id)::uuid
  AND schedule_id = sqlc.arg(schedule_id)::uuid
ORDER BY started_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountScheduleLogsBySchedule :one
SELECT count(*) FROM schedule_logs
WHERE team_id = sqlc.arg(team_id)::uuid
  AND schedule_id = sqlc.arg(schedule_id)::uuid;

-- name: DeleteScheduleLogsByBot :exec
DELETE FROM schedule_logs
WHERE team_id = sqlc.arg(team_id)::uuid
  AND bot_id = sqlc.arg(bot_id)::uuid;

-- name: DeleteScheduleLogsBySchedule :exec
DELETE FROM schedule_logs
WHERE team_id = sqlc.arg(team_id)::uuid
  AND schedule_id = sqlc.arg(schedule_id)::uuid;
