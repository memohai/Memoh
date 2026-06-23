-- name: UpsertMemoryNode :one
INSERT INTO memory_nodes (
  id, bot_id, body, hash, layer, fact_type, subject, confidence,
  metadata, source_message_ids, profile_ref, topic, captured_at, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
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
WHERE bot_id = $1 AND id = $2;

-- name: ListMemoryNodesByBot :many
SELECT * FROM memory_nodes
WHERE bot_id = $1
ORDER BY captured_at ASC;

-- name: ListMemoryNodesByBotLayer :many
SELECT * FROM memory_nodes
WHERE bot_id = $1 AND layer = $2
ORDER BY captured_at ASC;

-- name: ListMemoryNodesByBotProfile :many
SELECT * FROM memory_nodes
WHERE bot_id = $1 AND profile_ref = $2
ORDER BY captured_at ASC;

-- name: DeleteMemoryNode :exec
DELETE FROM memory_nodes
WHERE bot_id = $1 AND id = $2;

-- name: DeleteAllMemoryNodesByBot :exec
DELETE FROM memory_nodes
WHERE bot_id = $1;

-- name: CountMemoryNodesByBot :one
SELECT COUNT(*) FROM memory_nodes
WHERE bot_id = $1;

-- name: InsertMemoryEdge :exec
INSERT INTO memory_edges (bot_id, src_node, dst_node, rel, weight, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (bot_id, src_node, dst_node, rel) DO UPDATE SET
  weight = EXCLUDED.weight,
  metadata = EXCLUDED.metadata;

-- name: ListMemoryEdgesFromNode :many
SELECT * FROM memory_edges
WHERE bot_id = $1 AND src_node = $2
ORDER BY weight DESC;

-- name: ListMemoryEdgesByBot :many
SELECT * FROM memory_edges
WHERE bot_id = $1;

-- name: ListMemoryEdgesByRel :many
SELECT * FROM memory_edges
WHERE bot_id = $1 AND rel = $2;

-- name: DeleteMemoryEdgesForNode :exec
DELETE FROM memory_edges
WHERE bot_id = $1 AND (src_node = $2 OR dst_node = $2);

-- name: DeleteAllMemoryEdgesByBot :exec
DELETE FROM memory_edges
WHERE bot_id = $1;

-- name: CountMemoryEdgesByBot :one
SELECT COUNT(*) FROM memory_edges
WHERE bot_id = $1;

-- name: DeleteMemoryEdgesByRelForBot :exec
DELETE FROM memory_edges
WHERE bot_id = $1 AND rel = $2;

-- name: UpsertMemoryNodeEmbedding :exec
INSERT INTO memory_node_embeddings (
  bot_id, node_id, model_id, dimensions, body_hash, embedding
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.arg(node_id),
  sqlc.arg(model_id),
  sqlc.arg(dimensions),
  sqlc.arg(body_hash),
  sqlc.arg(embedding)
)
ON CONFLICT (bot_id, node_id, model_id) DO UPDATE SET
  dimensions = EXCLUDED.dimensions,
  body_hash = EXCLUDED.body_hash,
  embedding = EXCLUDED.embedding,
  updated_at = now();

-- name: SearchMemoryNodeEmbeddings :many
SELECT
  node_id,
  CAST(1.0 - (embedding <=> sqlc.arg(query_embedding)::vector) AS double precision) AS score
FROM memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id)
  AND model_id = sqlc.arg(model_id)
ORDER BY embedding <=> sqlc.arg(query_embedding)::vector
LIMIT sqlc.arg(row_limit);

-- name: CountMemoryNodeEmbeddingsByBotModel :one
SELECT COUNT(*) FROM memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id)
  AND model_id = sqlc.arg(model_id);

-- name: DeleteMemoryNodeEmbeddings :exec
DELETE FROM memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id)
  AND node_id = ANY(sqlc.arg(node_ids)::text[]);

-- name: DeleteAllMemoryNodeEmbeddingsByBot :exec
DELETE FROM memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id);

-- name: CheckMemoryNodeEmbeddingsStore :one
SELECT EXISTS (
  SELECT 1
  FROM memory_node_embeddings
  LIMIT 1
);
