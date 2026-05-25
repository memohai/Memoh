-- name: CreateTeamIssue :one
INSERT INTO team_issues (
  team_id, number, title, description, status,
  assignee_type, assignee_bot_id, assignee_user_id,
  created_by_type, created_by_bot_id, created_by_user_id,
  parent_issue_id, metadata
) VALUES (
  sqlc.arg(team_id),
  (SELECT COALESCE(MAX(number), 0) + 1 FROM team_issues WHERE team_id = sqlc.arg(team_id)),
  sqlc.arg(title),
  sqlc.arg(description),
  sqlc.arg(status),
  sqlc.narg(assignee_type),
  sqlc.narg(assignee_bot_id),
  sqlc.narg(assignee_user_id),
  sqlc.arg(created_by_type),
  sqlc.narg(created_by_bot_id),
  sqlc.narg(created_by_user_id),
  sqlc.narg(parent_issue_id),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetTeamIssue :one
SELECT * FROM team_issues WHERE id = $1;

-- name: GetTeamIssueInTeam :one
SELECT * FROM team_issues
WHERE id = sqlc.arg(id) AND team_id = sqlc.arg(team_id);

-- name: GetTeamIssueByNumber :one
SELECT * FROM team_issues
WHERE team_id = sqlc.arg(team_id) AND number = sqlc.arg(number);

-- name: ListTeamIssuesByTeam :many
SELECT * FROM team_issues
WHERE team_id = sqlc.arg(team_id)
ORDER BY updated_at DESC;

-- name: ListOpenTeamIssuesByTeam :many
SELECT * FROM team_issues
WHERE team_id = sqlc.arg(team_id)
  AND status NOT IN ('done', 'cancelled')
ORDER BY updated_at DESC;

-- name: ListTeamIssuesForOwner :many
SELECT i.* FROM team_issues i
JOIN agent_teams t ON t.id = i.team_id
WHERE t.owner_user_id = sqlc.arg(owner_user_id)
ORDER BY i.updated_at DESC;

-- name: UpdateTeamIssue :one
UPDATE team_issues SET
  title = COALESCE(sqlc.narg(title), title),
  description = COALESCE(sqlc.narg(description), description),
  status = COALESCE(sqlc.narg(status), status),
  assignee_type = COALESCE(sqlc.narg(assignee_type), assignee_type),
  assignee_bot_id = COALESCE(sqlc.narg(assignee_bot_id), assignee_bot_id),
  assignee_user_id = COALESCE(sqlc.narg(assignee_user_id), assignee_user_id),
  metadata = COALESCE(sqlc.narg(metadata), metadata),
  closed_at = CASE
    WHEN sqlc.narg(status)::text IN ('done', 'cancelled') AND closed_at IS NULL THEN now()
    WHEN sqlc.narg(status)::text IS NOT NULL AND sqlc.narg(status)::text NOT IN ('done', 'cancelled') THEN NULL
    ELSE closed_at
  END,
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetTeamIssueAssignee :one
UPDATE team_issues SET
  assignee_type = sqlc.narg(assignee_type),
  assignee_bot_id = sqlc.narg(assignee_bot_id),
  assignee_user_id = sqlc.narg(assignee_user_id),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteTeamIssue :exec
DELETE FROM team_issues WHERE id = $1;

-- name: CreateTeamIssueComment :one
INSERT INTO team_issue_comments (
  issue_id, team_id, parent_comment_id, author_type,
  author_bot_id, author_user_id, content, metadata
) VALUES (
  sqlc.arg(issue_id),
  sqlc.arg(team_id),
  sqlc.narg(parent_comment_id),
  sqlc.arg(author_type),
  sqlc.narg(author_bot_id),
  sqlc.narg(author_user_id),
  sqlc.arg(content),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetTeamIssueComment :one
SELECT * FROM team_issue_comments WHERE id = $1;

-- name: ListTeamIssueComments :many
SELECT * FROM team_issue_comments
WHERE issue_id = sqlc.arg(issue_id)
ORDER BY created_at ASC;

-- name: TouchTeamIssueAfterComment :exec
UPDATE team_issues SET updated_at = now() WHERE id = $1;

-- name: DeleteTeamIssueComment :exec
DELETE FROM team_issue_comments WHERE id = $1;
