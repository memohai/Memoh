-- 0129_skill_package_installations
-- Track materialized Skill Packages independently from their expanded Skills.

CREATE TABLE IF NOT EXISTS public.bot_skill_package_installations (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id            UUID        NOT NULL DEFAULT public.memoh_current_team_id()
                                   REFERENCES public.teams(id) ON DELETE RESTRICT,
    bot_id             UUID        NOT NULL,
    workspace_target_id TEXT       NOT NULL,
    registry_id        TEXT        NOT NULL,
    package_id         TEXT        NOT NULL,
    revision           TEXT        NOT NULL,
    directly_installed BOOLEAN     NOT NULL DEFAULT false,
    installed_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT memoh_team_key_2571b9adb696 UNIQUE (team_id, id),
    CONSTRAINT bot_skill_package_installations_identity_key
        UNIQUE (team_id, bot_id, workspace_target_id, registry_id, package_id),
    CONSTRAINT bot_skill_package_installations_bot_id_fkey
        FOREIGN KEY (team_id, bot_id)
        REFERENCES public.bots(team_id, id) ON DELETE CASCADE,
    CONSTRAINT bot_skill_package_installations_revision_check
        CHECK (revision ~ '^[0-9a-f]{64}$'),
    CONSTRAINT bot_skill_package_installations_registry_id_check
        CHECK (registry_id <> ''),
    CONSTRAINT bot_skill_package_installations_package_id_check
        CHECK (package_id <> '')
);

CREATE INDEX IF NOT EXISTS idx_bot_skill_package_installations_bot
    ON public.bot_skill_package_installations (team_id, bot_id, workspace_target_id);

CREATE TABLE IF NOT EXISTS public.bot_plugin_package_references (
    team_id               UUID        NOT NULL DEFAULT public.memoh_current_team_id()
                                     REFERENCES public.teams(id) ON DELETE RESTRICT,
    plugin_installation_id UUID       NOT NULL,
    package_installation_id UUID      NOT NULL,
    required_revision     TEXT        NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, plugin_installation_id, package_installation_id),
    CONSTRAINT bot_plugin_package_references_plugin_fkey
        FOREIGN KEY (team_id, plugin_installation_id)
        REFERENCES public.bot_plugin_installations(team_id, id) ON DELETE CASCADE,
    CONSTRAINT bot_plugin_package_references_package_fkey
        FOREIGN KEY (team_id, package_installation_id)
        REFERENCES public.bot_skill_package_installations(team_id, id) ON DELETE CASCADE,
    CONSTRAINT bot_plugin_package_references_revision_check
        CHECK (required_revision ~ '^[0-9a-f]{64}$')
);

CREATE INDEX IF NOT EXISTS idx_bot_plugin_package_references_package
    ON public.bot_plugin_package_references (team_id, package_installation_id);

ALTER TABLE public.bot_skill_package_installations ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bot_skill_package_installations FORCE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_package_references ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bot_plugin_package_references FORCE ROW LEVEL SECURITY;

CREATE POLICY bot_skill_package_installations_team_select ON public.bot_skill_package_installations
    FOR SELECT USING (team_id = public.memoh_current_team_id());
CREATE POLICY bot_skill_package_installations_team_insert ON public.bot_skill_package_installations
    FOR INSERT WITH CHECK (team_id = public.memoh_current_team_id());
CREATE POLICY bot_skill_package_installations_team_update ON public.bot_skill_package_installations
    FOR UPDATE
    USING (team_id = public.memoh_current_team_id())
    WITH CHECK (team_id = public.memoh_current_team_id());
CREATE POLICY bot_skill_package_installations_team_delete ON public.bot_skill_package_installations
    FOR DELETE USING (team_id = public.memoh_current_team_id());

CREATE POLICY bot_plugin_package_references_team_select ON public.bot_plugin_package_references
    FOR SELECT USING (team_id = public.memoh_current_team_id());
CREATE POLICY bot_plugin_package_references_team_insert ON public.bot_plugin_package_references
    FOR INSERT WITH CHECK (team_id = public.memoh_current_team_id());
CREATE POLICY bot_plugin_package_references_team_update ON public.bot_plugin_package_references
    FOR UPDATE
    USING (team_id = public.memoh_current_team_id())
    WITH CHECK (team_id = public.memoh_current_team_id());
CREATE POLICY bot_plugin_package_references_team_delete ON public.bot_plugin_package_references
    FOR DELETE USING (team_id = public.memoh_current_team_id());
