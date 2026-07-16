-- 0001_init
-- Create the optional pgvector semantic-memory index.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS public.memory_node_embeddings (
    bot_id      UUID        NOT NULL,
    node_id     TEXT        NOT NULL,
    model_id    UUID        NOT NULL,
    dimensions  INTEGER     NOT NULL,
    body_hash   TEXT        NOT NULL DEFAULT '',
    embedding   vector      NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, node_id, model_id),
    CONSTRAINT memory_node_embeddings_dimensions_check CHECK (dimensions > 0)
);

CREATE INDEX IF NOT EXISTS idx_memory_node_embeddings_bot_model
    ON public.memory_node_embeddings (bot_id, model_id);
