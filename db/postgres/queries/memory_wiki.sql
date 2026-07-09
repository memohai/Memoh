-- name: UpsertMemoryNode :one
INSERT INTO memory_nodes (
  team_id, id, bot_id, body, hash, layer, fact_type, subject, confidence,
  metadata, source_message_ids, profile_ref, topic, captured_at, expires_at
)
VALUES (
  sqlc.arg(team_id), sqlc.arg(id), sqlc.arg(bot_id), sqlc.arg(body),
  sqlc.arg(hash), sqlc.arg(layer), sqlc.arg(fact_type), sqlc.arg(subject),
  sqlc.arg(confidence), sqlc.arg(metadata), sqlc.arg(source_message_ids),
  sqlc.arg(profile_ref), sqlc.arg(topic), sqlc.arg(captured_at),
  sqlc.arg(expires_at)
)
ON CONFLICT (team_id, bot_id, id) DO UPDATE SET
  body = EXCLUDED.body,
  hash = EXCLUDED.hash,
  layer = EXCLUDED.layer,
  fact_type = EXCLUDED.fact_type,
  subject = EXCLUDED.subject,
  confidence = EXCLUDED.confidence,
  metadata = EXCLUDED.metadata,
  source_message_ids = EXCLUDED.source_message_ids,
  profile_ref = EXCLUDED.profile_ref,
  topic = EXCLUDED.topic,
  expires_at = EXCLUDED.expires_at,
  updated_at = now()
RETURNING *;

-- name: GetMemoryNode :one
SELECT * FROM memory_nodes
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND id = sqlc.arg(id);

-- name: ListMemoryNodesByBot :many
SELECT * FROM memory_nodes
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
ORDER BY captured_at ASC;

-- name: ListMemoryNodesByBotLayer :many
SELECT * FROM memory_nodes
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND layer = sqlc.arg(layer)
ORDER BY captured_at ASC;

-- name: ListMemoryNodesByBotProfile :many
SELECT * FROM memory_nodes
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND profile_ref = sqlc.arg(profile_ref)
ORDER BY captured_at ASC;

-- name: DeleteMemoryNode :exec
DELETE FROM memory_nodes
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND id = sqlc.arg(id);

-- name: DeleteAllMemoryNodesByBot :exec
DELETE FROM memory_nodes
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: CountMemoryNodesByBot :one
SELECT COUNT(*) FROM memory_nodes
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: InsertMemoryEdge :exec
INSERT INTO memory_edges (team_id, bot_id, src_node, dst_node, rel, weight, metadata)
VALUES (
  sqlc.arg(team_id), sqlc.arg(bot_id), sqlc.arg(src_node), sqlc.arg(dst_node),
  sqlc.arg(rel), sqlc.arg(weight), sqlc.arg(metadata)
)
ON CONFLICT (team_id, bot_id, src_node, dst_node, rel) DO UPDATE SET
  weight = EXCLUDED.weight,
  metadata = EXCLUDED.metadata;

-- name: ListMemoryEdgesFromNode :many
SELECT * FROM memory_edges
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND src_node = sqlc.arg(src_node)
ORDER BY weight DESC;

-- name: ListMemoryEdgesByBot :many
SELECT * FROM memory_edges
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: ListMemoryEdgesByRel :many
SELECT * FROM memory_edges
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND rel = sqlc.arg(rel);

-- name: DeleteMemoryEdgesForNode :exec
DELETE FROM memory_edges
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND (src_node = sqlc.arg(node_id) OR dst_node = sqlc.arg(node_id));

-- name: DeleteAllMemoryEdgesByBot :exec
DELETE FROM memory_edges
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: CountMemoryEdgesByBot :one
SELECT COUNT(*) FROM memory_edges
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id);

-- name: DeleteMemoryEdgesByRelForBot :exec
DELETE FROM memory_edges
WHERE team_id = sqlc.arg(team_id)
  AND bot_id = sqlc.arg(bot_id)
  AND rel = sqlc.arg(rel);
