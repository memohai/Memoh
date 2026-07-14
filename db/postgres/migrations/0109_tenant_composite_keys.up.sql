-- 0109_tenant_composite_keys
-- Atomically re-key every tenant business table on tenant_id: composite PKs,
-- tenant-scoped UNIQUE constraints/indexes, composite FKs (carrying tenant_id),
-- root FKs to tenants(id), and ON DELETE SET NULL -> RESTRICT.
--
-- New incremental (existing migrations untouched). This consolidates the schema
-- contract's UK / SET NULL->RESTRICT / composite PK+FK phases into ONE
-- transaction, because in PostgreSQL an FK binds to a specific unique/PK index:
-- you cannot rebuild a referenced key without dropping and recreating its
-- dependent FKs in the same operation. Splitting across migrations would fight
-- that coupling; doing it atomically is both correct and simpler to reason about.
--
-- Preconditions (from earlier migrations): every tenant table already has a
-- backfilled tenant_id (0108); the tenants root (0106) and app.tenant_write_fences
-- (0107) exist. Tenant tables are enumerated dynamically from the applied schema,
-- so this adapts to the fresh/legacy path divergence.
--
-- Algorithm:
--   Phase 1  record + drop all FKs among public tenant tables
--   Phase 2  rebuild PKs as (tenant_id, <old pk cols>)  [sets tenant_id NOT NULL]
--   Phase 3  rebuild UNIQUE constraints with tenant_id prepended
--   Phase 3b rebuild the partial/expression unique indexes (explicit)
--   Phase 4  recreate FKs as composite (tenant_id, col) -> parent(tenant_id, pcol),
--            converting SET NULL -> RESTRICT; add root FK (tenant_id)->tenants(id)
--
-- Note: all 104 tenant-table FKs are single-column at this base; the algorithm
-- assumes single-column business FKs (asserted below) and composite-izes them by
-- prepending tenant_id on both sides.

DO $$
DECLARE
    rec record;
    cols text;
    default_tenant constant uuid := '00000000-0000-0000-0000-000000000001';
BEGIN
    -- ----- scratch tables to carry FK definitions across phases -----
    CREATE TEMP TABLE _fk_saved ON COMMIT DROP AS
    SELECT con.oid,
           c.relname            AS child_table,
           con.conname          AS fk_name,
           rt.relname           AS parent_table,
           con.confdeltype      AS del_type,
           con.confupdtype      AS upd_type,
           (SELECT a.attname FROM pg_attribute a
             WHERE a.attrelid = con.conrelid AND a.attnum = con.conkey[1])  AS child_col,
           (SELECT a.attname FROM pg_attribute a
             WHERE a.attrelid = con.confrelid AND a.attnum = con.confkey[1]) AS parent_col,
           cardinality(con.conkey) AS ncols
      FROM pg_constraint con
      JOIN pg_class c  ON c.oid  = con.conrelid
      JOIN pg_class rt ON rt.oid = con.confrelid
      JOIN pg_namespace n ON n.oid = c.relnamespace
     WHERE con.contype = 'f'
       AND n.nspname = 'public'
       AND c.relname NOT IN ('schema_migrations', 'tenants');

    -- Safety: this algorithm only handles single-column business FKs.
    IF EXISTS (SELECT 1 FROM _fk_saved WHERE ncols <> 1) THEN
        RAISE EXCEPTION 'multi-column FK present; composite re-key algorithm needs revision';
    END IF;

    -- Persist original FK delete actions so the down migration can restore them
    -- exactly (the SET NULL -> RESTRICT conversion below is otherwise lossy: the
    -- original action is not recoverable from the mutated schema alone). This
    -- backup lives in the app schema and is dropped by the down migration.
    CREATE TABLE IF NOT EXISTS app.tenant_fk_original (
        child_table  text NOT NULL,
        fk_name      text NOT NULL,
        parent_table text NOT NULL,
        child_col    text NOT NULL,
        parent_col   text NOT NULL,
        del_type     "char" NOT NULL,
        upd_type     "char" NOT NULL,
        PRIMARY KEY (child_table, fk_name)
    );
    INSERT INTO app.tenant_fk_original
        (child_table, fk_name, parent_table, child_col, parent_col, del_type, upd_type)
    SELECT child_table, fk_name, parent_table, child_col, parent_col, del_type, upd_type
      FROM _fk_saved
    ON CONFLICT (child_table, fk_name) DO NOTHING;

    -- ===== Phase 1: drop all recorded FKs =====
    FOR rec IN SELECT child_table, fk_name FROM _fk_saved LOOP
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.child_table, rec.fk_name);
    END LOOP;

    -- ===== Phase 2: rebuild PKs as (tenant_id, <old pk cols>) =====
    FOR rec IN
        SELECT c.relname AS table_name, con.conname, con.conrelid, con.conkey
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE con.contype = 'p' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
    LOOP
        -- skip if already tenant-scoped (idempotent re-run)
        IF (SELECT attname FROM pg_attribute
              WHERE attrelid = rec.conrelid AND attnum = rec.conkey[1]) = 'tenant_id' THEN
            CONTINUE;
        END IF;
        SELECT 'tenant_id, ' || string_agg(quote_ident(a.attname), ', ' ORDER BY k.ord)
          INTO cols
          FROM unnest(rec.conkey) WITH ORDINALITY AS k(attnum, ord)
          JOIN pg_attribute a ON a.attrelid = rec.conrelid AND a.attnum = k.attnum;
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.table_name, rec.conname);
        EXECUTE format('ALTER TABLE public.%I ADD CONSTRAINT %I PRIMARY KEY (%s)',
                       rec.table_name, rec.conname, cols);
    END LOOP;

    -- ===== Phase 3: rebuild UNIQUE constraints with tenant_id prepended =====
    FOR rec IN
        SELECT c.relname AS table_name, con.conname, con.conrelid, con.conkey,
               i.indnullsnotdistinct AS nulls_not_distinct
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
          JOIN pg_index i ON i.indexrelid = con.conindid
         WHERE con.contype = 'u' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
    LOOP
        IF (SELECT attname FROM pg_attribute
              WHERE attrelid = rec.conrelid AND attnum = rec.conkey[1]) = 'tenant_id' THEN
            CONTINUE;
        END IF;
        SELECT 'tenant_id, ' || string_agg(quote_ident(a.attname), ', ' ORDER BY k.ord)
          INTO cols
          FROM unnest(rec.conkey) WITH ORDINALITY AS k(attnum, ord)
          JOIN pg_attribute a ON a.attrelid = rec.conrelid AND a.attnum = k.attnum;
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.table_name, rec.conname);
        -- Preserve NULLS NOT DISTINCT: a bare UNIQUE would widen the semantics
        -- (NULLs become distinct), letting duplicate rows with NULL key columns
        -- through (e.g. bot_acl_rules_unique_target).
        EXECUTE format('ALTER TABLE public.%I ADD CONSTRAINT %I UNIQUE %s (%s)',
                       rec.table_name, rec.conname,
                       CASE WHEN rec.nulls_not_distinct THEN 'NULLS NOT DISTINCT' ELSE '' END,
                       cols);
    END LOOP;

    -- ===== Phase 4: recreate FKs as composite, SET NULL -> RESTRICT =====
    FOR rec IN SELECT * FROM _fk_saved LOOP
        EXECUTE format(
            'ALTER TABLE public.%I ADD CONSTRAINT %I FOREIGN KEY (tenant_id, %I) '
            || 'REFERENCES public.%I (tenant_id, %I) ON UPDATE %s ON DELETE %s',
            rec.child_table, rec.fk_name, rec.child_col,
            rec.parent_table, rec.parent_col,
            CASE rec.upd_type WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT'
                              WHEN 'n' THEN 'RESTRICT' WHEN 'd' THEN 'RESTRICT' ELSE 'NO ACTION' END,
            -- SET NULL (n) -> RESTRICT; keep CASCADE (c); RESTRICT (r); else NO ACTION
            CASE rec.del_type WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT'
                              WHEN 'n' THEN 'RESTRICT' WHEN 'd' THEN 'RESTRICT' ELSE 'NO ACTION' END
        );
    END LOOP;

    -- ===== Phase 4b: add root FK (tenant_id) -> tenants(id) on every table =====
    FOR rec IN
        SELECT c.relname AS table_name
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind = 'r' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
    LOOP
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint con
              JOIN pg_class rt ON rt.oid = con.confrelid
             WHERE con.contype = 'f'
               AND con.conrelid = ('public.'||quote_ident(rec.table_name))::regclass
               AND rt.relname = 'tenants'
        ) THEN
            EXECUTE format(
                'ALTER TABLE public.%I ADD CONSTRAINT %I FOREIGN KEY (tenant_id) '
                || 'REFERENCES public.tenants (id) ON DELETE RESTRICT',
                rec.table_name, rec.table_name || '_tenant_id_fkey');
        END IF;
    END LOOP;
END
$$;

-- Lock down the FK-backup table: owned by memoh_owner, no runtime/PUBLIC access.
-- It exists only so the down migration can restore exact FK actions; runtime
-- must never see it.
ALTER TABLE app.tenant_fk_original OWNER TO memoh_owner;
REVOKE ALL ON TABLE app.tenant_fk_original FROM PUBLIC, memoh_runtime;

-- ===== Phase 3b: partial / expression unique indexes with tenant_id prepended =====
DROP INDEX IF EXISTS idx_bot_channel_external_identity;
CREATE UNIQUE INDEX idx_bot_channel_external_identity
    ON public.bot_channel_configs (tenant_id, channel_type, external_identity);

DROP INDEX IF EXISTS idx_bot_channel_routes_unique;
CREATE UNIQUE INDEX idx_bot_channel_routes_unique
    ON public.bot_channel_routes
       (tenant_id, bot_id, channel_type, external_conversation_id, COALESCE(external_thread_id, ''::text));

DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
CREATE UNIQUE INDEX idx_bot_history_messages_turn_seq_unique
    ON public.bot_history_messages (tenant_id, turn_id, turn_message_seq)
    WHERE ((turn_id IS NOT NULL) AND (turn_message_seq IS NOT NULL));

DROP INDEX IF EXISTS idx_bot_user_grants_unique_everyone;
CREATE UNIQUE INDEX idx_bot_user_grants_unique_everyone
    ON public.bot_user_grants (tenant_id, bot_id)
    WHERE (subject_type = 'everyone'::text);

DROP INDEX IF EXISTS idx_bot_user_grants_unique_user;
CREATE UNIQUE INDEX idx_bot_user_grants_unique_user
    ON public.bot_user_grants (tenant_id, bot_id, user_id)
    WHERE (subject_type = 'user'::text);

DROP INDEX IF EXISTS idx_bots_name;
CREATE UNIQUE INDEX idx_bots_name
    ON public.bots (tenant_id, name);

DROP INDEX IF EXISTS idx_session_events_dedup;
CREATE UNIQUE INDEX idx_session_events_dedup
    ON public.bot_session_events (tenant_id, session_id, event_kind, external_message_id)
    WHERE ((external_message_id IS NOT NULL) AND (external_message_id <> ''::text));

DROP INDEX IF EXISTS idx_snapshots_container_runtime_name;
CREATE UNIQUE INDEX idx_snapshots_container_runtime_name
    ON public.snapshots (tenant_id, container_id, runtime_snapshot_name);
