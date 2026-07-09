-- name: InsertLifecycleEvent :exec
INSERT INTO lifecycle_events (id, container_id, event_type, payload)
SELECT
  sqlc.arg(id),
  c.container_id,
  sqlc.arg(event_type),
  sqlc.arg(payload)
FROM containers c
WHERE c.container_id = sqlc.arg(container_id)
  AND c.team_id = sqlc.arg(team_id);
