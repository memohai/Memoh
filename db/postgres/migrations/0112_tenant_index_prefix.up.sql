-- 0112_tenant_index_prefix
-- Prepend tenant_id to every non-unique btree secondary index on a tenant table
-- that does not already lead with it. Tenant queries filter by tenant_id (RLS +
-- explicit app.current_tenant_id()), so a tenant_id-leading index lets the
-- planner scan only the current tenant's slice. Purely a performance change.
--
-- New incremental (existing migrations untouched). Each index is dropped and
-- recreated with tenant_id prepended, preserving its column list, partial WHERE,
-- and (btree) access method. Non-btree (gin/gist) indexes are left untouched.

DO $$
DECLARE
    rec record;
    idxdef text;
BEGIN
    FOR rec IN
        SELECT ic.relname AS index_name, pg_get_indexdef(i.indexrelid) AS def
          FROM pg_index i
          JOIN pg_class ic ON ic.oid = i.indexrelid
          JOIN pg_class tc ON tc.oid = i.indrelid
          JOIN pg_namespace n ON n.oid = tc.relnamespace
          JOIN pg_am am ON am.oid = ic.relam
         WHERE n.nspname = 'public'
           AND NOT i.indisprimary AND NOT i.indisunique
           AND am.amname = 'btree'
           AND EXISTS (SELECT 1 FROM pg_attribute
                        WHERE attrelid = i.indrelid AND attname = 'tenant_id')
           AND (SELECT attname FROM pg_attribute
                 WHERE attrelid = i.indrelid AND attnum = i.indkey[0]) <> 'tenant_id'
    LOOP
        idxdef := regexp_replace(rec.def, '(USING btree \()', '\1tenant_id, ');
        EXECUTE format('DROP INDEX IF EXISTS public.%I', rec.index_name);
        EXECUTE idxdef;
    END LOOP;
END
$$;
