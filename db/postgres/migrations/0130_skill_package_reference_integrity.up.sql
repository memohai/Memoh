-- 0130_skill_package_reference_integrity
-- Make Plugin target and Package ownership relationships queryable and enforceable.

ALTER TABLE public.bot_plugin_installations
    ADD COLUMN workspace_target_id TEXT NOT NULL DEFAULT 'native';

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
    ADD CONSTRAINT bot_skill_package_installations_team_id_revision_key
    UNIQUE (team_id, id, revision);

ALTER TABLE public.bot_plugin_package_references
    ADD COLUMN bot_id UUID;

UPDATE public.bot_plugin_package_references AS reference
SET bot_id = plugin.bot_id
FROM public.bot_plugin_installations AS plugin
WHERE plugin.team_id = reference.team_id
  AND plugin.id = reference.plugin_installation_id;

ALTER TABLE public.bot_plugin_package_references
    ALTER COLUMN bot_id SET NOT NULL,
    DROP CONSTRAINT bot_plugin_package_references_plugin_fkey,
    DROP CONSTRAINT bot_plugin_package_references_package_fkey,
    ADD CONSTRAINT bot_plugin_package_references_plugin_fkey
        FOREIGN KEY (team_id, plugin_installation_id, bot_id)
        REFERENCES public.bot_plugin_installations(team_id, id, bot_id) ON DELETE CASCADE,
    ADD CONSTRAINT bot_plugin_package_references_package_fkey
        FOREIGN KEY (team_id, package_installation_id, bot_id)
        REFERENCES public.bot_skill_package_installations(team_id, id, bot_id) ON DELETE CASCADE,
    ADD CONSTRAINT bot_plugin_package_references_revision_fkey
        FOREIGN KEY (team_id, package_installation_id, required_revision)
        REFERENCES public.bot_skill_package_installations(team_id, id, revision) ON DELETE CASCADE;
