-- 0002_team_isolation
-- Remove team scoping from the semantic-memory embeddings table.

DROP POLICY IF EXISTS memory_node_embeddings_team_delete
    ON public.memory_node_embeddings;
DROP POLICY IF EXISTS memory_node_embeddings_team_update
    ON public.memory_node_embeddings;
DROP POLICY IF EXISTS memory_node_embeddings_team_insert
    ON public.memory_node_embeddings;
DROP POLICY IF EXISTS memory_node_embeddings_team_select
    ON public.memory_node_embeddings;

ALTER TABLE public.memory_node_embeddings NO FORCE ROW LEVEL SECURITY;
ALTER TABLE public.memory_node_embeddings DISABLE ROW LEVEL SECURITY;

DROP INDEX IF EXISTS public.idx_memory_node_embeddings_team_bot_model;

DO $memory_embeddings_pk$
DECLARE
    current_pk_name TEXT;
BEGIN
    SELECT conname
      INTO current_pk_name
      FROM pg_constraint
     WHERE conrelid = 'public.memory_node_embeddings'::regclass
       AND contype = 'p';

    IF current_pk_name IS NOT NULL THEN
        EXECUTE format(
            'ALTER TABLE public.memory_node_embeddings DROP CONSTRAINT %I',
            current_pk_name
        );
    END IF;

    ALTER TABLE public.memory_node_embeddings
        ADD CONSTRAINT memory_node_embeddings_pkey
        PRIMARY KEY (bot_id, node_id, model_id);
END
$memory_embeddings_pk$;

ALTER TABLE public.memory_node_embeddings
    DROP COLUMN IF EXISTS team_id;

CREATE INDEX IF NOT EXISTS idx_memory_node_embeddings_bot_model
    ON public.memory_node_embeddings (bot_id, model_id);

DROP FUNCTION IF EXISTS public.memoh_pgvector_current_team_id();
