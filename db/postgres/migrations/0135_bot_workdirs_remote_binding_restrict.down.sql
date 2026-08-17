-- 0135_bot_workdirs_remote_binding_restrict
-- Restore cascading deletion of workdirs when a remote workspace binding is removed.

ALTER TABLE public.bot_workdirs
    DROP CONSTRAINT IF EXISTS bot_workdirs_remote_binding_fkey;

-- Existing rows were protected by the replaced RESTRICT constraint, so the
-- restored constraint can be installed fully validated.
ALTER TABLE public.bot_workdirs
    ADD CONSTRAINT bot_workdirs_remote_binding_fkey
    FOREIGN KEY (team_id, remote_binding_id)
    REFERENCES public.bot_remote_runtime_bindings(team_id, id) ON DELETE CASCADE;
