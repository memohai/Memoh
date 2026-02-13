-- name: InsertSnapshot :exec
INSERT INTO snapshots (id, container_id, parent_snapshot_id, snapshotter, digest)
VALUES (
  sqlc.arg(id),
  sqlc.arg(container_id),
  sqlc.arg(parent_snapshot_id),
  sqlc.arg(snapshotter),
  sqlc.arg(digest)
)
ON CONFLICT (id) DO NOTHING;
