-- name: UpsertMemoryNodeEmbedding :exec
INSERT INTO public.memory_node_embeddings (
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
  CAST(1.0 - (embedding <=> sqlc.arg(embedding)::vector) AS double precision) AS score
FROM public.memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id)
  AND model_id = sqlc.arg(model_id)
ORDER BY embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(row_limit);

-- name: DeleteMemoryNodeEmbeddings :exec
DELETE FROM public.memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id)
  AND node_id = ANY(sqlc.arg(node_ids)::text[]);

-- name: DeleteBotMemoryNodeEmbeddings :exec
DELETE FROM public.memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id);

-- name: CountMemoryNodeEmbeddings :one
SELECT COUNT(*)
FROM public.memory_node_embeddings
WHERE bot_id = sqlc.arg(bot_id)
  AND model_id = sqlc.arg(model_id);

-- name: MemoryNodeEmbeddingsExist :one
SELECT EXISTS (
  SELECT 1
  FROM public.memory_node_embeddings
  LIMIT 1
);
