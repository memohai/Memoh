-- name: ListVersionsByContainerID :many
SELECT
  cv.id, cv.container_id, cv.snapshot_id, cv.version, cv.created_at,
  s.runtime_snapshot_name, s.display_name
FROM container_versions cv
JOIN snapshots s ON s.id = cv.snapshot_id
WHERE cv.container_id = sqlc.arg(container_id)
ORDER BY cv.version ASC;

-- name: NextVersion :one
SELECT COALESCE(MAX(version), 0) + 1 FROM container_versions WHERE container_id = sqlc.arg(container_id);

-- name: InsertVersion :one
INSERT INTO container_versions (id, container_id, snapshot_id, version)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(container_id),
  sqlc.arg(snapshot_id),
  sqlc.arg(version)
)
RETURNING id, container_id, snapshot_id, version, created_at;

-- name: GetVersionSnapshotRuntimeName :one
SELECT s.runtime_snapshot_name
FROM container_versions cv
JOIN snapshots s ON s.id = cv.snapshot_id
WHERE cv.container_id = sqlc.arg(container_id)
  AND cv.version = sqlc.arg(version);
