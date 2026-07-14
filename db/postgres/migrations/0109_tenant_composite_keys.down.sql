-- 0109_tenant_composite_keys (down)
-- Reverse the atomic composite re-key: restore single-column PKs/UKs/FKs and
-- the original ON DELETE SET NULL semantics for the FKs that had them.
--
-- Down safety gate: only safe on a clean singleton database (no non-default
-- tenant rows anywhere). If multiple tenants share natural keys, restoring a
-- global PK/UNIQUE would collide; we surface that as the natural error rather
-- than destroying data. This matches the schema contract's down safety gate:
-- down to the pre-tenant single-column shape may restore the old single-column
-- delete semantics (including SET NULL), but only when the data is a clean
-- singleton.
--
-- The original per-FK delete actions were persisted by the up migration into
-- app.tenant_fk_original, so the down restores them EXACTLY (the SET NULL ->
-- RESTRICT conversion is otherwise lossy). We do not guess from nullability.

DO $$
DECLARE
    rec record;
    default_tenant constant uuid := '00000000-0000-0000-0000-000000000001';
    orig_del text;
    orig_upd text;
BEGIN
    -- Fail-closed gate: refuse if any tenant table holds a non-default tenant_id.
    FOR rec IN
        SELECT c.relname
          FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind = 'r' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
           AND EXISTS (SELECT 1 FROM information_schema.columns
                        WHERE table_schema='public' AND table_name=c.relname AND column_name='tenant_id')
    LOOP
        EXECUTE format('SELECT 1 FROM public.%I WHERE tenant_id <> %L LIMIT 1', rec.relname, default_tenant);
        IF FOUND THEN
            RAISE EXCEPTION 'refusing composite-key rollback: % has non-default tenant rows', rec.relname;
        END IF;
    END LOOP;

    -- Drop all business + root FKs on tenant tables.
    FOR rec IN
        SELECT c.relname AS child_table, con.conname AS fk_name
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE con.contype = 'f' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
    LOOP
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.child_table, rec.fk_name);
    END LOOP;

    -- Restore single-column PKs (strip leading tenant_id).
    FOR rec IN
        SELECT c.relname AS table_name, con.conname, con.conrelid, con.conkey
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE con.contype = 'p' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
    LOOP
        IF (SELECT attname FROM pg_attribute WHERE attrelid=rec.conrelid AND attnum=rec.conkey[1]) <> 'tenant_id' THEN
            CONTINUE;
        END IF;
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.table_name, rec.conname);
        EXECUTE format('ALTER TABLE public.%I ADD CONSTRAINT %I PRIMARY KEY (%s)',
            rec.table_name, rec.conname,
            (SELECT string_agg(quote_ident(a.attname), ', ' ORDER BY k.ord)
               FROM unnest(rec.conkey) WITH ORDINALITY AS k(attnum, ord)
               JOIN pg_attribute a ON a.attrelid=rec.conrelid AND a.attnum=k.attnum
              WHERE k.ord > 1));
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
           AND c.relname NOT IN ('schema_migrations', 'tenants')
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

    -- Recreate single-column FKs with their EXACT original delete/update action.
    FOR rec IN SELECT * FROM app.tenant_fk_original LOOP
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

    DROP TABLE IF EXISTS app.tenant_fk_original;
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
