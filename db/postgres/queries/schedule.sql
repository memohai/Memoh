-- name: CreateSchedule :one
INSERT INTO schedule (team_id, name, description, pattern, max_calls, enabled, command, bot_id)
VALUES (sqlc.arg(team_id)::uuid, sqlc.arg(name), sqlc.arg(description), sqlc.arg(pattern), sqlc.arg(max_calls), sqlc.arg(enabled), sqlc.arg(command), sqlc.arg(bot_id)::uuid)
RETURNING *;

-- name: GetScheduleByID :one
SELECT *
FROM schedule
WHERE id = sqlc.arg(id)::uuid
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: ListSchedulesByBot :many
SELECT *
FROM schedule
WHERE team_id = sqlc.arg(team_id)::uuid
  AND bot_id = sqlc.arg(bot_id)::uuid
ORDER BY created_at DESC;

-- name: ListEnabledSchedules :many
-- Process-wide startup bootstrap: intentionally spans all teams (no team_id
-- filter) so every team's cron jobs are registered after a restart. Each row
-- carries team_id, which the scheduler threads into the job it starts.
SELECT *
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
    updated_at = now()
WHERE id = sqlc.arg(id)::uuid
  AND team_id = sqlc.arg(team_id)::uuid
RETURNING *;

-- name: DeleteSchedule :exec
DELETE FROM schedule
WHERE id = sqlc.arg(id)::uuid
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: IncrementScheduleCalls :one
UPDATE schedule
SET current_calls = current_calls + 1,
    enabled = CASE
      WHEN max_calls IS NOT NULL AND current_calls + 1 >= max_calls THEN false
      ELSE enabled
    END,
    updated_at = now()
WHERE id = sqlc.arg(id)::uuid
  AND team_id = sqlc.arg(team_id)::uuid
RETURNING *;
