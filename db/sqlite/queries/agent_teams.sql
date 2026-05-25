-- name: CreateAgentTeam :one
INSERT INTO agent_teams (id, owner_user_id, name, description, shared_dir_name, instructions, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(owner_user_id),
  sqlc.arg(name),
  sqlc.arg(description),
  sqlc.arg(shared_dir_name),
  sqlc.arg(instructions),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetAgentTeam :one
SELECT * FROM agent_teams WHERE id = sqlc.arg(id);

-- name: GetAgentTeamForOwner :one
SELECT * FROM agent_teams
WHERE id = sqlc.arg(id) AND owner_user_id = sqlc.arg(owner_user_id);

-- name: ListAgentTeamsByOwner :many
SELECT * FROM agent_teams
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND archived_at IS NULL
ORDER BY created_at DESC;

-- name: ListAllAgentTeamsByOwner :many
SELECT * FROM agent_teams
WHERE owner_user_id = sqlc.arg(owner_user_id)
ORDER BY created_at DESC;

-- name: ListAllAgentTeams :many
SELECT * FROM agent_teams
WHERE archived_at IS NULL
ORDER BY created_at DESC;

-- name: UpdateAgentTeam :one
UPDATE agent_teams SET
  name = COALESCE(sqlc.narg(name), name),
  description = COALESCE(sqlc.narg(description), description),
  shared_dir_name = COALESCE(sqlc.narg(shared_dir_name), shared_dir_name),
  instructions = COALESCE(sqlc.narg(instructions), instructions),
  metadata = COALESCE(sqlc.narg(metadata), metadata),
  updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ArchiveAgentTeam :one
UPDATE agent_teams SET archived_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteAgentTeam :exec
DELETE FROM agent_teams WHERE id = sqlc.arg(id);

-- name: ListAgentTeamsForBot :many
SELECT t.* FROM agent_teams t
JOIN agent_team_members m ON m.team_id = t.id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND t.archived_at IS NULL
ORDER BY t.created_at DESC;

-- name: AddAgentTeamMember :one
INSERT INTO agent_team_members (id, team_id, member_type, bot_id, user_id, role, instructions, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(team_id),
  sqlc.arg(member_type),
  sqlc.narg(bot_id),
  sqlc.narg(user_id),
  sqlc.arg(role),
  sqlc.arg(instructions),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: ListAgentTeamMembers :many
SELECT
  m.id, m.team_id, m.member_type, m.bot_id, m.user_id,
  m.role, m.instructions, m.metadata, m.created_at, m.updated_at,
  COALESCE(b.display_name, u.display_name, '') AS resolved_display_name
FROM agent_team_members m
LEFT JOIN bots b ON m.bot_id = b.id
LEFT JOIN users u ON m.user_id = u.id
WHERE m.team_id = sqlc.arg(team_id)
ORDER BY m.created_at ASC;

-- name: GetAgentTeamMember :one
SELECT
  m.id, m.team_id, m.member_type, m.bot_id, m.user_id,
  m.role, m.instructions, m.metadata, m.created_at, m.updated_at,
  COALESCE(b.display_name, u.display_name, '') AS resolved_display_name
FROM agent_team_members m
LEFT JOIN bots b ON m.bot_id = b.id
LEFT JOIN users u ON m.user_id = u.id
WHERE m.id = sqlc.arg(id);

-- name: GetAgentTeamMemberByBot :one
SELECT
  m.id, m.team_id, m.member_type, m.bot_id, m.user_id,
  m.role, m.instructions, m.metadata, m.created_at, m.updated_at,
  COALESCE(b.display_name, '') AS resolved_display_name
FROM agent_team_members m
LEFT JOIN bots b ON m.bot_id = b.id
WHERE m.team_id = sqlc.arg(team_id) AND m.bot_id = sqlc.arg(bot_id);

-- name: GetAgentTeamMemberByUser :one
SELECT
  m.id, m.team_id, m.member_type, m.bot_id, m.user_id,
  m.role, m.instructions, m.metadata, m.created_at, m.updated_at,
  COALESCE(u.display_name, '') AS resolved_display_name
FROM agent_team_members m
LEFT JOIN users u ON m.user_id = u.id
WHERE m.team_id = sqlc.arg(team_id) AND m.user_id = sqlc.arg(user_id);

-- name: UpdateAgentTeamMember :one
UPDATE agent_team_members SET
  role = COALESCE(sqlc.narg(role), role),
  instructions = COALESCE(sqlc.narg(instructions), instructions),
  metadata = COALESCE(sqlc.narg(metadata), metadata),
  updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteAgentTeamMember :exec
DELETE FROM agent_team_members WHERE id = sqlc.arg(id);
