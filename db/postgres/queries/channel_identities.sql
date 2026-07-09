-- name: CreateChannelIdentity :one
INSERT INTO channel_identities (team_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES (sqlc.arg(team_id), sqlc.arg(channel_type), sqlc.arg(channel_subject_id), sqlc.arg(display_name), sqlc.arg(avatar_url), sqlc.arg(metadata))
RETURNING *;

-- name: GetChannelIdentityByID :one
SELECT *
FROM channel_identities
WHERE team_id = sqlc.arg(team_id)
  AND id = sqlc.arg(id);

-- name: GetChannelIdentityByIDForUpdate :one
SELECT *
FROM channel_identities
WHERE team_id = sqlc.arg(team_id)
  AND id = sqlc.arg(id)
FOR UPDATE;

-- name: GetChannelIdentityByChannelSubject :one
SELECT *
FROM channel_identities
WHERE team_id = sqlc.arg(team_id)
  AND channel_type = sqlc.arg(channel_type)
  AND channel_subject_id = sqlc.arg(channel_subject_id);

-- name: UpsertChannelIdentityByChannelSubject :one
INSERT INTO channel_identities (team_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES (sqlc.arg(team_id), sqlc.arg(channel_type), sqlc.arg(channel_subject_id), sqlc.arg(display_name), sqlc.arg(avatar_url), sqlc.arg(metadata))
ON CONFLICT (team_id, channel_type, channel_subject_id)
DO UPDATE SET
  display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), channel_identities.display_name),
  avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), channel_identities.avatar_url),
  metadata = EXCLUDED.metadata,
  updated_at = now()
RETURNING *;

-- name: SearchChannelIdentities :many
SELECT ci.*
FROM channel_identities ci
WHERE team_id = sqlc.arg(team_id)
  AND (
    sqlc.arg(query)::text = ''
    OR ci.channel_type ILIKE '%' || sqlc.arg(query)::text || '%'
    OR ci.channel_subject_id ILIKE '%' || sqlc.arg(query)::text || '%'
    OR COALESCE(ci.display_name, '') ILIKE '%' || sqlc.arg(query)::text || '%'
  )
ORDER BY ci.updated_at DESC
LIMIT sqlc.arg(limit_count);
