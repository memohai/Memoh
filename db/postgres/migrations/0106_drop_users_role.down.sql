-- 0106_drop_users_role (down)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
    CREATE TYPE user_role AS ENUM ('member', 'admin');
  END IF;
END $$;

ALTER TABLE IF EXISTS users ADD COLUMN IF NOT EXISTS role user_role NOT NULL DEFAULT 'member';

UPDATE users u
SET role = 'admin'
WHERE EXISTS (
  SELECT 1 FROM team_members m
  WHERE m.user_id = u.id AND m.role IN ('owner', 'admin')
);
