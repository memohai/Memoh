-- name: GetBotSkillPackageInstallation :one
SELECT id, team_id, bot_id, workspace_target_id, registry_id, package_id,
       revision, directly_installed, installed_at, updated_at
FROM bot_skill_package_installations
WHERE team_id = public.memoh_current_team_id()
  AND bot_id = $1
  AND workspace_target_id = $2
  AND registry_id = $3
  AND package_id = $4
LIMIT 1;

-- name: GetBotSkillPackageInstallationByID :one
SELECT id, team_id, bot_id, workspace_target_id, registry_id, package_id,
       revision, directly_installed, installed_at, updated_at
FROM bot_skill_package_installations
WHERE team_id = public.memoh_current_team_id()
  AND bot_id = $1
  AND id = $2
LIMIT 1;

-- name: ListBotSkillPackageInstallations :many
SELECT id, team_id, bot_id, workspace_target_id, registry_id, package_id,
       revision, directly_installed, installed_at, updated_at,
       (
         SELECT count(*)
         FROM bot_plugin_package_references AS ref
         WHERE ref.team_id = public.memoh_current_team_id()
           AND ref.package_installation_id = installation.id
       ) AS plugin_reference_count
FROM bot_skill_package_installations AS installation
WHERE installation.team_id = public.memoh_current_team_id() AND installation.bot_id = $1
ORDER BY installation.registry_id, installation.package_id, installation.workspace_target_id;

-- name: UpsertDirectBotSkillPackageInstallation :one
INSERT INTO bot_skill_package_installations (
  bot_id, workspace_target_id, registry_id, package_id, revision, directly_installed
)
VALUES ($1, $2, $3, $4, $5, true)
ON CONFLICT (team_id, bot_id, workspace_target_id, registry_id, package_id)
DO UPDATE SET revision = EXCLUDED.revision,
              directly_installed = true,
              updated_at = now()
WHERE bot_skill_package_installations.revision = EXCLUDED.revision
   OR NOT EXISTS (
     SELECT 1
     FROM bot_plugin_package_references AS ref
     WHERE ref.team_id = public.memoh_current_team_id()
       AND ref.package_installation_id = bot_skill_package_installations.id
       AND ref.required_revision <> EXCLUDED.revision
   )
RETURNING id, team_id, bot_id, workspace_target_id, registry_id, package_id,
          revision, directly_installed, installed_at, updated_at;

-- name: UpsertPluginBotSkillPackageInstallation :one
INSERT INTO bot_skill_package_installations (
  bot_id, workspace_target_id, registry_id, package_id, revision, directly_installed
)
VALUES ($1, $2, $3, $4, $5, false)
ON CONFLICT (team_id, bot_id, workspace_target_id, registry_id, package_id)
DO UPDATE SET revision = EXCLUDED.revision,
              updated_at = now()
WHERE (
  bot_skill_package_installations.revision = EXCLUDED.revision
  OR (
    NOT bot_skill_package_installations.directly_installed
    AND NOT EXISTS (
      SELECT 1
      FROM bot_plugin_package_references AS ref
      WHERE ref.team_id = public.memoh_current_team_id()
        AND ref.package_installation_id = bot_skill_package_installations.id
        AND ref.required_revision <> EXCLUDED.revision
    )
  )
)
RETURNING id, team_id, bot_id, workspace_target_id, registry_id, package_id,
          revision, directly_installed, installed_at, updated_at;

-- name: SetBotSkillPackageDirectlyInstalled :one
UPDATE bot_skill_package_installations
SET directly_installed = $3,
    updated_at = now()
WHERE team_id = public.memoh_current_team_id() AND bot_id = $1 AND id = $2
RETURNING id, team_id, bot_id, workspace_target_id, registry_id, package_id,
          revision, directly_installed, installed_at, updated_at;

-- name: DeleteBotSkillPackageInstallationIfUnreferenced :one
DELETE FROM bot_skill_package_installations AS installation
WHERE installation.team_id = public.memoh_current_team_id()
  AND installation.bot_id = $1
  AND installation.id = $2
  AND NOT installation.directly_installed
  AND NOT EXISTS (
    SELECT 1
    FROM bot_plugin_package_references AS ref
    WHERE ref.team_id = public.memoh_current_team_id()
      AND ref.package_installation_id = installation.id
  )
RETURNING installation.id;

-- name: UpsertBotPluginPackageReference :one
INSERT INTO bot_plugin_package_references (
  bot_id, workspace_target_id, plugin_installation_id, package_installation_id, required_revision
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (team_id, plugin_installation_id, package_installation_id)
DO UPDATE SET required_revision = EXCLUDED.required_revision,
              updated_at = now()
RETURNING *;

-- name: ListBotPluginPackageReferences :many
SELECT ref.team_id, ref.bot_id, ref.workspace_target_id, ref.plugin_installation_id, ref.package_installation_id,
       ref.required_revision, ref.created_at, ref.updated_at,
       installation.registry_id, installation.package_id,
       installation.revision, installation.directly_installed
FROM bot_plugin_package_references AS ref
JOIN bot_skill_package_installations AS installation
  ON installation.team_id = ref.team_id AND installation.id = ref.package_installation_id
WHERE ref.team_id = public.memoh_current_team_id()
  AND ref.plugin_installation_id = $1
ORDER BY installation.registry_id, installation.package_id;

-- name: DeleteBotPluginPackageReferences :exec
DELETE FROM bot_plugin_package_references
WHERE team_id = public.memoh_current_team_id() AND plugin_installation_id = $1;

-- name: CountBotSkillPackageReferences :one
SELECT count(*)
FROM bot_plugin_package_references
WHERE team_id = public.memoh_current_team_id() AND package_installation_id = $1;
