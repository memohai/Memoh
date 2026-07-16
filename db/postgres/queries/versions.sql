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
WHERE cv.team_id = public.memoh_current_team_id() AND s.team_id = public.memoh_current_team_id() AND cv.container_id = sqlc.arg(container_id)
ORDER BY cv.version ASC;

-- name: NextVersion :one
SELECT COALESCE(MAX(version), 0) + 1 FROM container_versions WHERE team_id = public.memoh_current_team_id() AND container_id = sqlc.arg(container_id);

-- name: InsertVersion :one
INSERT INTO container_versions (container_id, snapshot_id, version)
VALUES (
  sqlc.arg(container_id),
  sqlc.arg(snapshot_id),
  sqlc.arg(version)
)
RETURNING id, container_id, snapshot_id, version, created_at, team_id;

-- name: GetVersionSnapshotRuntimeName :one
SELECT s.runtime_snapshot_name
FROM container_versions cv
JOIN snapshots s ON s.id = cv.snapshot_id
WHERE cv.team_id = public.memoh_current_team_id() AND s.team_id = public.memoh_current_team_id() AND cv.container_id = sqlc.arg(container_id)
  AND cv.version = sqlc.arg(version);
