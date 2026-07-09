-- 0106_drop_users_role
-- Authority is owned by team_members.role. Remove the global users.role flag
-- and its enum; users stays a global identity table.
ALTER TABLE IF EXISTS users DROP COLUMN IF EXISTS role;
DROP TYPE IF EXISTS user_role;
