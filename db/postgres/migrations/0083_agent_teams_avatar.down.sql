-- 0083_agent_teams_avatar
-- Drop avatar_url column.

ALTER TABLE agent_teams DROP COLUMN IF EXISTS avatar_url;
