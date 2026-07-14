-- 0109_tenant_composite_keys (down)
-- Reverse the tenant-scoped unique keys and composite foreign keys.
--
-- Down safety gate: only safe on a clean singleton database (no non-default
-- tenant rows anywhere). If multiple tenants share natural keys, restoring a
-- global UNIQUE constraints could collide. The down migration therefore only
-- restores the pre-tenant single-column shape and delete semantics when the
-- data is still a clean singleton.
--
DO $$
DECLARE
    rec record;
    default_tenant constant uuid := '00000000-0000-0000-0000-000000000001';
    orig_del text;
    orig_upd text;
    has_non_default boolean;
BEGIN
    CREATE TEMP TABLE _tenant_tables (table_name text PRIMARY KEY) ON COMMIT DROP;
    INSERT INTO _tenant_tables
    SELECT c.relname
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
      JOIN pg_attribute a ON a.attrelid = c.oid
      JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
     WHERE n.nspname = 'public'
       AND c.relkind IN ('r', 'p')
       AND a.attname = 'tenant_id'
       AND NOT a.attisdropped
       AND pg_get_expr(d.adbin, d.adrelid) LIKE '%app.current_tenant_id()%';

    -- Fail-closed gate: refuse if any tenant table holds a non-default tenant_id.
    FOR rec IN SELECT table_name FROM _tenant_tables LOOP
        EXECUTE format(
            'SELECT EXISTS (SELECT 1 FROM public.%I WHERE tenant_id <> %L)',
            rec.table_name, default_tenant
        ) INTO has_non_default;
        IF has_non_default THEN
            RAISE EXCEPTION 'refusing tenant-key rollback: % has non-default tenant rows', rec.table_name;
        END IF;
    END LOOP;

    -- Save the current composite business FKs. Their action codes still carry
    -- the exact pre-tenant semantics, including column-scoped SET NULL.
    CREATE TEMP TABLE _fk_saved ON COMMIT DROP AS
    SELECT c.relname AS child_table,
           con.conname AS fk_name,
           rt.relname AS parent_table,
           con.confdeltype AS del_type,
           con.confupdtype AS upd_type,
           (SELECT a.attname FROM pg_attribute a
             WHERE a.attrelid = con.conrelid AND a.attnum = con.conkey[2]) AS child_col,
           (SELECT a.attname FROM pg_attribute a
             WHERE a.attrelid = con.confrelid AND a.attnum = con.confkey[2]) AS parent_col
      FROM pg_constraint con
      JOIN pg_class c ON c.oid = con.conrelid
      JOIN pg_class rt ON rt.oid = con.confrelid
      JOIN pg_namespace n ON n.oid = c.relnamespace
     WHERE con.contype = 'f'
       AND n.nspname = 'public'
       AND c.relname IN (SELECT table_name FROM _tenant_tables)
       AND rt.relname IN (SELECT table_name FROM _tenant_tables)
       AND cardinality(con.conkey) = 2
       AND (SELECT a.attname FROM pg_attribute a
             WHERE a.attrelid = con.conrelid AND a.attnum = con.conkey[1]) = 'tenant_id';

    -- Drop the composite business FKs and tenant-root FKs only.
    FOR rec IN
        SELECT c.relname AS child_table, con.conname AS fk_name
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE con.contype = 'f' AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
           AND EXISTS (
               SELECT 1 FROM pg_attribute a
                WHERE a.attrelid = con.conrelid
                  AND a.attnum = ANY(con.conkey)
                  AND a.attname = 'tenant_id'
           )
    LOOP
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.child_table, rec.fk_name);
    END LOOP;

    -- Drop the helper unique keys added for unchanged primary keys.
    FOR rec IN
        SELECT c.relname AS table_name, con.conname
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE con.contype = 'u' AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
           AND con.conname LIKE 'memoh_tenant_key_%'
    LOOP
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.table_name, rec.conname);
    END LOOP;

    -- Restore single-column UNIQUE constraints (strip leading tenant_id).
    FOR rec IN
        SELECT c.relname AS table_name, con.conname, con.conrelid, con.conkey,
               i.indnullsnotdistinct AS nulls_not_distinct
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
          JOIN pg_index i ON i.indexrelid = con.conindid
         WHERE con.contype = 'u' AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
           AND con.conname NOT LIKE 'memoh_tenant_key_%'
    LOOP
        IF (SELECT attname FROM pg_attribute WHERE attrelid=rec.conrelid AND attnum=rec.conkey[1]) <> 'tenant_id' THEN
            CONTINUE;
        END IF;
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.table_name, rec.conname);
        EXECUTE format('ALTER TABLE public.%I ADD CONSTRAINT %I UNIQUE %s (%s)',
            rec.table_name, rec.conname,
            CASE WHEN rec.nulls_not_distinct THEN 'NULLS NOT DISTINCT' ELSE '' END,
            (SELECT string_agg(quote_ident(a.attname), ', ' ORDER BY k.ord)
               FROM unnest(rec.conkey) WITH ORDINALITY AS k(attnum, ord)
               JOIN pg_attribute a ON a.attrelid=rec.conrelid AND a.attnum=k.attnum
              WHERE k.ord > 1));
    END LOOP;

    -- Recreate the original single-column FKs with their exact actions.
    FOR rec IN SELECT * FROM _fk_saved LOOP
        orig_del := CASE rec.del_type WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT'
                                      WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT'
                                      ELSE 'NO ACTION' END;
        orig_upd := CASE rec.upd_type WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT'
                                      WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT'
                                      ELSE 'NO ACTION' END;
        EXECUTE format(
            'ALTER TABLE public.%I ADD CONSTRAINT %I FOREIGN KEY (%I) REFERENCES public.%I (%I) ON UPDATE %s ON DELETE %s',
            rec.child_table, rec.fk_name, rec.child_col, rec.parent_table, rec.parent_col, orig_upd, orig_del);
    END LOOP;

END
$$;

-- Restore the partial / expression unique indexes to their original (global) shape.
DROP INDEX IF EXISTS idx_bot_channel_external_identity;
CREATE UNIQUE INDEX idx_bot_channel_external_identity
    ON public.bot_channel_configs (channel_type, external_identity);

DROP INDEX IF EXISTS idx_bot_channel_routes_unique;
CREATE UNIQUE INDEX idx_bot_channel_routes_unique
    ON public.bot_channel_routes
       (bot_id, channel_type, external_conversation_id, COALESCE(external_thread_id, ''::text));

DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
CREATE UNIQUE INDEX idx_bot_history_messages_turn_seq_unique
    ON public.bot_history_messages (turn_id, turn_message_seq)
    WHERE ((turn_id IS NOT NULL) AND (turn_message_seq IS NOT NULL));

DROP INDEX IF EXISTS idx_bot_user_grants_unique_everyone;
CREATE UNIQUE INDEX idx_bot_user_grants_unique_everyone
    ON public.bot_user_grants (bot_id)
    WHERE (subject_type = 'everyone'::text);

DROP INDEX IF EXISTS idx_bot_user_grants_unique_user;
CREATE UNIQUE INDEX idx_bot_user_grants_unique_user
    ON public.bot_user_grants (bot_id, user_id)
    WHERE (subject_type = 'user'::text);

DROP INDEX IF EXISTS idx_bots_name;
CREATE UNIQUE INDEX idx_bots_name
    ON public.bots (name);

DROP INDEX IF EXISTS idx_session_events_dedup;
CREATE UNIQUE INDEX idx_session_events_dedup
    ON public.bot_session_events (session_id, event_kind, external_message_id)
    WHERE ((external_message_id IS NOT NULL) AND (external_message_id <> ''::text));

DROP INDEX IF EXISTS idx_snapshots_container_runtime_name;
CREATE UNIQUE INDEX idx_snapshots_container_runtime_name
    ON public.snapshots (container_id, runtime_snapshot_name);
