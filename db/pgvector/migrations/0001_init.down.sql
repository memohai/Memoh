-- 0001_init
-- Remove the optional pgvector semantic-memory index.

DROP TABLE IF EXISTS public.memory_node_embeddings;
DROP EXTENSION IF EXISTS vector;
