-- name: UpsertSnapshot :one
INSERT INTO snapshots (
  container_id,
  runtime_snapshot_name,
  parent_runtime_snapshot_name,
  snapshotter,
  source
)
VALUES (
  sqlc.arg(container_id),
  sqlc.arg(runtime_snapshot_name),
  sqlc.arg(parent_runtime_snapshot_name),
  sqlc.arg(snapshotter),
  sqlc.arg(source)
)
ON CONFLICT (container_id, runtime_snapshot_name) DO UPDATE
SET
  parent_runtime_snapshot_name = EXCLUDED.parent_runtime_snapshot_name,
  snapshotter = EXCLUDED.snapshotter,
  source = EXCLUDED.source
RETURNING id, container_id, runtime_snapshot_name, parent_runtime_snapshot_name, snapshotter, source, created_at;

-- name: ListSnapshotsByContainerID :many
SELECT
  id,
  container_id,
  runtime_snapshot_name,
  parent_runtime_snapshot_name,
  snapshotter,
  source,
  created_at
FROM snapshots
WHERE container_id = sqlc.arg(container_id)
ORDER BY created_at DESC;

-- name: ListSnapshotsWithVersionByContainerID :many
SELECT
  s.id,
  s.container_id,
  s.runtime_snapshot_name,
  s.parent_runtime_snapshot_name,
  s.snapshotter,
  s.source,
  s.created_at,
  cv.version
FROM snapshots s
LEFT JOIN container_versions cv ON cv.snapshot_id = s.id
WHERE s.container_id = sqlc.arg(container_id)
ORDER BY s.created_at DESC;

-- name: GetSnapshotByContainerAndRuntimeName :one
SELECT
  id,
  container_id,
  runtime_snapshot_name,
  parent_runtime_snapshot_name,
  snapshotter,
  source,
  created_at
FROM snapshots
WHERE container_id = sqlc.arg(container_id)
  AND runtime_snapshot_name = sqlc.arg(runtime_snapshot_name)
LIMIT 1;
