-- 0109_tenant_composite_keys
-- Add tenant-scoped unique keys and composite foreign keys while preserving the
-- existing primary keys and delete behavior.
--
-- This work is atomic because in PostgreSQL an FK binds to a specific unique
-- index: you cannot rebuild a referenced key without dropping and recreating its
-- dependent FKs in the same operation. Splitting across migrations would fight
-- that coupling; doing it atomically is both correct and simpler to reason about.
--
-- Preconditions (from earlier migrations): every tenant table already has a
-- backfilled tenant_id (0108), and the tenants root (0106) exists. Tenant tables
-- are identified by the tenant default installed by 0108, so user-managed
-- tables in the public schema are not modified.
--
-- Existing ON DELETE SET NULL constraints use PostgreSQL's column-list form,
-- SET NULL (child_column), so the reference is cleared without clearing the
-- non-null tenant_id column.

DO $$
DECLARE
    rec record;
    cols text;
    delete_action text;
    update_action text;
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
       AND c.relname IN (SELECT table_name FROM _tenant_tables)
       AND rt.relname IN (SELECT table_name FROM _tenant_tables);

    -- Safety: this algorithm only handles single-column business FKs.
    IF EXISTS (SELECT 1 FROM _fk_saved WHERE ncols <> 1) THEN
        RAISE EXCEPTION 'multi-column FK present; composite re-key algorithm needs revision';
    END IF;

    IF EXISTS (SELECT 1 FROM _fk_saved WHERE upd_type IN ('n', 'd')) THEN
        RAISE EXCEPTION 'ON UPDATE SET NULL/DEFAULT requires an explicit tenant-safe migration';
    END IF;

    FOR rec IN SELECT child_table, fk_name FROM _fk_saved LOOP
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.child_table, rec.fk_name);
    END LOOP;

    -- Keep existing primary keys stable. Add a tenant-prefixed unique key for
    -- each primary key so composite foreign keys have a valid target.
    FOR rec IN
        SELECT c.relname AS table_name, con.conname, con.conrelid, con.conkey
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE con.contype = 'p' AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
    LOOP
        SELECT 'tenant_id, ' || string_agg(quote_ident(a.attname), ', ' ORDER BY k.ord)
          INTO cols
          FROM unnest(rec.conkey) WITH ORDINALITY AS k(attnum, ord)
          JOIN pg_attribute a ON a.attrelid = rec.conrelid AND a.attnum = k.attnum;
        EXECUTE format('ALTER TABLE public.%I ALTER COLUMN tenant_id SET NOT NULL', rec.table_name);
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint
             WHERE conrelid = rec.conrelid
               AND conname = 'memoh_tenant_key_' || substr(md5(rec.table_name || ':' || rec.conname), 1, 12)
        ) THEN
            EXECUTE format('ALTER TABLE public.%I ADD CONSTRAINT %I UNIQUE (%s)',
                rec.table_name,
                'memoh_tenant_key_' || substr(md5(rec.table_name || ':' || rec.conname), 1, 12),
                cols);
        END IF;
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
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
           AND con.conname NOT LIKE 'memoh_tenant_key_%'
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

    FOR rec IN SELECT * FROM _fk_saved LOOP
        update_action := CASE rec.upd_type WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT'
                          ELSE 'NO ACTION' END;
        delete_action := CASE rec.del_type
            WHEN 'c' THEN 'CASCADE'
            WHEN 'r' THEN 'RESTRICT'
            WHEN 'n' THEN format('SET NULL (%I)', rec.child_col)
            WHEN 'd' THEN format('SET DEFAULT (%I)', rec.child_col)
            ELSE 'NO ACTION'
        END;
        EXECUTE format(
            'ALTER TABLE public.%I ADD CONSTRAINT %I FOREIGN KEY (tenant_id, %I) '
            || 'REFERENCES public.%I (tenant_id, %I) ON UPDATE %s ON DELETE %s',
            rec.child_table, rec.fk_name, rec.child_col,
            rec.parent_table, rec.parent_col,
            update_action, delete_action
        );
    END LOOP;

    -- ===== Phase 4b: add root FK (tenant_id) -> tenants(id) on every table =====
    FOR rec IN
        SELECT c.relname AS table_name
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind IN ('r', 'p') AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
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
