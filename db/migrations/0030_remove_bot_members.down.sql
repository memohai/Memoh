-- 0030_remove_bot_members
-- Restore bot member sharing model.

CREATE TABLE IF NOT EXISTS bot_members (
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_members_role_check CHECK (role IN ('owner', 'admin', 'member')),
  CONSTRAINT bot_members_unique UNIQUE (bot_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_members_user_id ON bot_members(user_id);
