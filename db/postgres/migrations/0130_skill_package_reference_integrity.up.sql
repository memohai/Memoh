-- 0130_skill_package_reference_integrity
-- Make Plugin target and Package ownership relationships queryable and enforceable.

-- Existing installations are protected by FORCE ROW LEVEL SECURITY, but this
-- migration must backfill rows without a request-scoped team context. Restore
-- the policies after the data changes, matching the existing migration pattern.
ALTER TABLE public.bot_plugin_installations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_installations DISABLE ROW LEVEL SECURITY;
ALTER TABLE public.bot_skill_package_installations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE public.bot_skill_package_installations DISABLE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_package_references NO FORCE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_package_references DISABLE ROW LEVEL SECURITY;

ALTER TABLE public.bot_plugin_installations
    ADD COLUMN workspace_target_id TEXT NOT NULL DEFAULT 'native',
    ADD CONSTRAINT bot_plugin_installations_workspace_target_id_check
        CHECK (workspace_target_id <> '' AND workspace_target_id = btrim(workspace_target_id));

UPDATE public.bot_plugin_installations
SET workspace_target_id = btrim(metadata ->> 'workspace_target_id')
WHERE metadata ? 'workspace_target_id'
  AND btrim(metadata ->> 'workspace_target_id') <> '';

ALTER TABLE public.bot_plugin_installations
    ADD CONSTRAINT bot_plugin_installations_team_id_id_bot_id_key
    UNIQUE (team_id, id, bot_id);

ALTER TABLE public.bot_skill_package_installations
    ADD CONSTRAINT bot_skill_package_installations_team_id_bot_id_key
    UNIQUE (team_id, id, bot_id),
    ADD CONSTRAINT bot_skill_package_installations_team_id_bot_id_target_key
    UNIQUE (team_id, id, bot_id, workspace_target_id),
    ADD CONSTRAINT bot_skill_package_installations_team_id_revision_key
    UNIQUE (team_id, id, revision),
    ADD CONSTRAINT bot_skill_package_installations_workspace_target_id_check
        CHECK (workspace_target_id <> '' AND workspace_target_id = btrim(workspace_target_id));

ALTER TABLE public.bot_plugin_package_references
    ADD COLUMN bot_id UUID,
    ADD COLUMN workspace_target_id TEXT;

UPDATE public.bot_plugin_package_references AS reference
SET bot_id = package.bot_id,
    workspace_target_id = package.workspace_target_id
FROM public.bot_skill_package_installations AS package
WHERE package.team_id = reference.team_id
  AND package.id = reference.package_installation_id;

ALTER TABLE public.bot_plugin_package_references
    ALTER COLUMN bot_id SET NOT NULL,
    ALTER COLUMN workspace_target_id SET NOT NULL,
    DROP CONSTRAINT bot_plugin_package_references_plugin_fkey,
    DROP CONSTRAINT bot_plugin_package_references_package_fkey,
    ADD CONSTRAINT bot_plugin_package_references_plugin_fkey
        FOREIGN KEY (team_id, plugin_installation_id, bot_id)
        REFERENCES public.bot_plugin_installations(team_id, id, bot_id) ON DELETE CASCADE,
    ADD CONSTRAINT bot_plugin_package_references_package_fkey
        FOREIGN KEY (team_id, package_installation_id, bot_id, workspace_target_id)
        REFERENCES public.bot_skill_package_installations(team_id, id, bot_id, workspace_target_id) ON DELETE CASCADE,
    ADD CONSTRAINT bot_plugin_package_references_revision_fkey
        FOREIGN KEY (team_id, package_installation_id, required_revision)
        REFERENCES public.bot_skill_package_installations(team_id, id, revision) ON DELETE CASCADE;

ALTER TABLE public.bot_plugin_installations ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_installations FORCE ROW LEVEL SECURITY;
ALTER TABLE public.bot_skill_package_installations ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bot_skill_package_installations FORCE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_package_references ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_package_references FORCE ROW LEVEL SECURITY;
