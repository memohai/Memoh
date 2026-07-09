-- name: ListVersionsByContainerID :many
SELECT
  cv.id,
  cv.container_id,
  cv.snapshot_id,
  cv.version,
  cv.created_at,
  s.runtime_snapshot_name,
  s.display_name
FROM container_versions cv
JOIN snapshots s ON s.id = cv.snapshot_id
JOIN containers c ON c.container_id = cv.container_id
WHERE cv.container_id = sqlc.arg(container_id)
  AND c.team_id = sqlc.arg(team_id)
ORDER BY cv.version ASC;

-- name: NextVersion :one
SELECT COALESCE(MAX(cv.version), 0) + 1
FROM container_versions cv
JOIN containers c ON c.container_id = cv.container_id
WHERE cv.container_id = sqlc.arg(container_id)
  AND c.team_id = sqlc.arg(team_id);

-- name: InsertVersion :one
INSERT INTO container_versions (container_id, snapshot_id, version, team_id)
SELECT
  c.container_id,
  s.id,
  sqlc.arg(version),
  c.team_id
FROM containers c
JOIN snapshots s ON s.id = sqlc.arg(snapshot_id)
WHERE c.container_id = sqlc.arg(container_id)
  AND c.team_id = sqlc.arg(team_id)
  AND s.container_id = c.container_id
  AND s.team_id = c.team_id
RETURNING *;

-- name: GetVersionSnapshotRuntimeName :one
SELECT s.runtime_snapshot_name
FROM container_versions cv
JOIN snapshots s ON s.id = cv.snapshot_id
JOIN containers c ON c.container_id = cv.container_id
WHERE cv.container_id = sqlc.arg(container_id)
  AND cv.version = sqlc.arg(version)
  AND c.team_id = sqlc.arg(team_id);
