-- 0130_skill_package_reference_integrity

ALTER TABLE public.bot_plugin_package_references
    DROP CONSTRAINT bot_plugin_package_references_revision_fkey,
    DROP CONSTRAINT bot_plugin_package_references_package_fkey,
    DROP CONSTRAINT bot_plugin_package_references_plugin_fkey,
    ADD CONSTRAINT bot_plugin_package_references_plugin_fkey
        FOREIGN KEY (team_id, plugin_installation_id)
        REFERENCES public.bot_plugin_installations(team_id, id) ON DELETE CASCADE,
    ADD CONSTRAINT bot_plugin_package_references_package_fkey
        FOREIGN KEY (team_id, package_installation_id)
        REFERENCES public.bot_skill_package_installations(team_id, id) ON DELETE CASCADE,
    DROP COLUMN workspace_target_id,
    DROP COLUMN bot_id;

ALTER TABLE public.bot_skill_package_installations
    DROP CONSTRAINT bot_skill_package_installations_workspace_target_id_check,
    DROP CONSTRAINT bot_skill_package_installations_team_id_revision_key,
    DROP CONSTRAINT bot_skill_package_installations_team_id_bot_id_target_key,
    DROP CONSTRAINT bot_skill_package_installations_team_id_bot_id_key;

ALTER TABLE public.bot_plugin_installations
    DROP CONSTRAINT bot_plugin_installations_team_id_id_bot_id_key,
    DROP CONSTRAINT bot_plugin_installations_workspace_target_id_check,
    DROP COLUMN workspace_target_id;
