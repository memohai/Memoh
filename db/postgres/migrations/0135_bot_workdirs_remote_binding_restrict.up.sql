-- 0135_bot_workdirs_remote_binding_restrict
-- Keep remote workspace bindings while any live or archived workdir refers to them.

ALTER TABLE public.bot_workdirs
    DROP CONSTRAINT IF EXISTS bot_workdirs_remote_binding_fkey;

-- The replaced CASCADE constraint already proved all existing rows valid, so
-- the replacement can be installed as a fully validated constraint.
ALTER TABLE public.bot_workdirs
    ADD CONSTRAINT bot_workdirs_remote_binding_fkey
    FOREIGN KEY (team_id, remote_binding_id)
    REFERENCES public.bot_remote_runtime_bindings(team_id, id) ON DELETE RESTRICT;
