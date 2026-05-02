-- 0077_memory_sql_index
-- Remove database-backed memory vector indexes.

DROP TABLE IF EXISTS memory_sparse_terms;
DROP TABLE IF EXISTS memory_dense_vectors;
DROP TABLE IF EXISTS memory_index_points;
