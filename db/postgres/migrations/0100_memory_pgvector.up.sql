-- 0100_memory_pgvector
-- Store optional semantic memory seeds in Postgres via pgvector.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memory_node_embeddings (
    bot_id      UUID        NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    node_id     TEXT        NOT NULL REFERENCES memory_nodes(id) ON DELETE CASCADE,
    model_id    UUID        NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    dimensions  INTEGER     NOT NULL,
    body_hash   TEXT        NOT NULL DEFAULT '',
    embedding   vector      NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, node_id, model_id),
    CONSTRAINT memory_node_embeddings_dimensions_check CHECK (dimensions > 0)
);

CREATE INDEX IF NOT EXISTS idx_memory_node_embeddings_bot_model ON memory_node_embeddings (bot_id, model_id);
