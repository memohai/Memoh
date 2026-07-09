-- name: UpsertSnapshot :one
INSERT INTO snapshots (
  container_id,
  team_id,
  runtime_snapshot_name,
  display_name,
  parent_runtime_snapshot_name,
  snapshotter,
  source
)
SELECT
  c.container_id,
  c.team_id,
  sqlc.arg(runtime_snapshot_name),
  sqlc.arg(display_name),
  sqlc.arg(parent_runtime_snapshot_name),
  sqlc.arg(snapshotter),
  sqlc.arg(source)
FROM containers c
WHERE c.container_id = sqlc.arg(container_id)
  AND c.team_id = sqlc.arg(team_id)
ON CONFLICT (container_id, runtime_snapshot_name) DO UPDATE
SET
  display_name = EXCLUDED.display_name,
  parent_runtime_snapshot_name = EXCLUDED.parent_runtime_snapshot_name,
  snapshotter = EXCLUDED.snapshotter,
  source = EXCLUDED.source
WHERE EXISTS (
  SELECT 1
  FROM containers c
  WHERE c.container_id = snapshots.container_id
    AND c.team_id = sqlc.arg(team_id)
)
RETURNING *;

-- name: ListSnapshotsByContainerID :many
SELECT snapshots.*
FROM snapshots
JOIN containers c ON c.container_id = snapshots.container_id
WHERE snapshots.container_id = sqlc.arg(container_id)
  AND c.team_id = sqlc.arg(team_id)
ORDER BY snapshots.created_at DESC;

-- name: ListSnapshotsWithVersionByContainerID :many
SELECT
  s.id,
  s.container_id,
  s.runtime_snapshot_name,
  s.display_name,
  s.parent_runtime_snapshot_name,
  s.snapshotter,
  s.source,
  s.created_at,
  cv.version
FROM snapshots s
LEFT JOIN container_versions cv ON cv.snapshot_id = s.id
JOIN containers c ON c.container_id = s.container_id
WHERE s.container_id = sqlc.arg(container_id)
  AND c.team_id = sqlc.arg(team_id)
ORDER BY s.created_at DESC;

-- name: GetSnapshotByContainerAndRuntimeName :one
SELECT snapshots.*
FROM snapshots
JOIN containers c ON c.container_id = snapshots.container_id
WHERE snapshots.container_id = sqlc.arg(container_id)
  AND snapshots.runtime_snapshot_name = sqlc.arg(runtime_snapshot_name)
  AND c.team_id = sqlc.arg(team_id)
LIMIT 1;
