-- 0029_chat_acl
-- Restore allow_guest storage and preauth table, then drop bot ACL rules.

ALTER TABLE bots ADD COLUMN IF NOT EXISTS allow_guest BOOLEAN NOT NULL DEFAULT false;

UPDATE bots
SET allow_guest = true
WHERE type = 'public'
  AND EXISTS (
    SELECT 1
    FROM bot_acl_rules r
    WHERE r.bot_id = bots.id
      AND r.action = 'chat.trigger'
      AND r.effect = 'allow'
      AND r.subject_kind = 'guest_all'
  );

CREATE TABLE IF NOT EXISTS bot_preauth_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  token TEXT NOT NULL,
  issued_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  expires_at TIMESTAMPTZ,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_preauth_keys_unique UNIQUE (token)
);

CREATE INDEX IF NOT EXISTS idx_bot_preauth_keys_bot_id ON bot_preauth_keys(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_preauth_keys_expires ON bot_preauth_keys(expires_at);

DROP TABLE IF EXISTS bot_acl_rules;
