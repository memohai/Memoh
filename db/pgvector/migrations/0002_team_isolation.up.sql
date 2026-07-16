-- 0002_team_isolation
-- Scope semantic-memory embeddings to a team and enforce fail-closed RLS.

CREATE OR REPLACE FUNCTION public.memoh_pgvector_current_team_id()
RETURNS UUID
LANGUAGE plpgsql
STABLE
SECURITY INVOKER
SET search_path = pg_catalog, pg_temp
AS $current_team$
DECLARE
    raw TEXT;
BEGIN
    raw := pg_catalog.current_setting('memoh.team_id', true);
    IF raw IS NULL OR pg_catalog.btrim(raw) = '' THEN
        RAISE EXCEPTION 'memoh.team_id is not set'
            USING ERRCODE = '42501';
    END IF;
    BEGIN
        RETURN raw::UUID;
    EXCEPTION
        WHEN invalid_text_representation THEN
            RAISE EXCEPTION 'memoh.team_id is invalid'
                USING ERRCODE = '22P02';
    END;
END;
$current_team$;

ALTER TABLE public.memory_node_embeddings
    ADD COLUMN IF NOT EXISTS team_id UUID;

UPDATE public.memory_node_embeddings
SET team_id = '00000000-0000-0000-0000-000000000001'
WHERE team_id IS NULL;

ALTER TABLE public.memory_node_embeddings
    ALTER COLUMN team_id SET DEFAULT public.memoh_pgvector_current_team_id(),
    ALTER COLUMN team_id SET NOT NULL;

DO $memory_embeddings_pk$
DECLARE
    current_pk_name TEXT;
    current_pk_def  TEXT;
BEGIN
    SELECT conname, pg_get_constraintdef(oid)
      INTO current_pk_name, current_pk_def
      FROM pg_constraint
     WHERE conrelid = 'public.memory_node_embeddings'::regclass
       AND contype = 'p';

    IF current_pk_name IS NOT NULL
       AND current_pk_def <> 'PRIMARY KEY (team_id, bot_id, node_id, model_id)' THEN
        EXECUTE format(
            'ALTER TABLE public.memory_node_embeddings DROP CONSTRAINT %I',
            current_pk_name
        );
        current_pk_name := NULL;
    END IF;

    IF current_pk_name IS NULL THEN
        ALTER TABLE public.memory_node_embeddings
            ADD CONSTRAINT memory_node_embeddings_pkey
            PRIMARY KEY (team_id, bot_id, node_id, model_id);
    END IF;
END
$memory_embeddings_pk$;

DROP INDEX IF EXISTS public.idx_memory_node_embeddings_bot_model;
CREATE INDEX IF NOT EXISTS idx_memory_node_embeddings_team_bot_model
    ON public.memory_node_embeddings (team_id, bot_id, model_id);

ALTER TABLE public.memory_node_embeddings ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.memory_node_embeddings FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS memory_node_embeddings_team_select
    ON public.memory_node_embeddings;
CREATE POLICY memory_node_embeddings_team_select
    ON public.memory_node_embeddings
    FOR SELECT
    USING (team_id = public.memoh_pgvector_current_team_id());

DROP POLICY IF EXISTS memory_node_embeddings_team_insert
    ON public.memory_node_embeddings;
CREATE POLICY memory_node_embeddings_team_insert
    ON public.memory_node_embeddings
    FOR INSERT
    WITH CHECK (team_id = public.memoh_pgvector_current_team_id());

DROP POLICY IF EXISTS memory_node_embeddings_team_update
    ON public.memory_node_embeddings;
CREATE POLICY memory_node_embeddings_team_update
    ON public.memory_node_embeddings
    FOR UPDATE
    USING (team_id = public.memoh_pgvector_current_team_id())
    WITH CHECK (team_id = public.memoh_pgvector_current_team_id());

DROP POLICY IF EXISTS memory_node_embeddings_team_delete
    ON public.memory_node_embeddings;
CREATE POLICY memory_node_embeddings_team_delete
    ON public.memory_node_embeddings
    FOR DELETE
    USING (team_id = public.memoh_pgvector_current_team_id());
