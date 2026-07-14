-- 0112_tenant_index_prefix (down)
-- Rebuild the tenant_id-leading secondary indexes back to their original form
-- (strip the leading tenant_id column). Only affects btree indexes whose first
-- column is tenant_id and whose second column onward is the original definition.

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
           AND (SELECT attname FROM pg_attribute
                 WHERE attrelid = i.indrelid AND attnum = i.indkey[0]) = 'tenant_id'
           AND array_length(i.indkey, 1) > 1
    LOOP
        idxdef := regexp_replace(rec.def, '(USING btree \()tenant_id, ', '\1');
        EXECUTE format('DROP INDEX IF EXISTS public.%I', rec.index_name);
        EXECUTE idxdef;
    END LOOP;
END
$$;
