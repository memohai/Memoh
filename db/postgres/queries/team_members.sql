-- name: GetTeamMembership :one
-- Returns the caller's role in a team. Both args are explicit (team + user);
-- this is the membership gate lookup, not an auto-team-scoped query.
SELECT team_id, user_id, role
FROM team_members
WHERE team_id = sqlc.arg(team_id)::uuid
  AND user_id = sqlc.arg(user_id)::uuid;
