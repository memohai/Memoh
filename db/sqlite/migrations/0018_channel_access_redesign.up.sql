-- 0018_channel_access_redesign
-- Channel Access redesign: per-bot channel-identity Manage grants plus global
-- web-user <-> IM channel-identity account binding (with one-time link codes).

-- bot_channel_admins: channel-identity level Manage grant. Lets an IM identity
-- run owner-only slash commands (e.g. /model set) without being a web user.
-- granted carries the local override state for the Manage capability:
--   1 -> locally granted (force ON)
--   0 -> locally suppressed (force OFF, overrides an inherited grant)
-- No row -> fall back to the inherited grant (bound web user is owner / has manage).
CREATE TABLE IF NOT EXISTS bot_channel_admins (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  channel_identity_id TEXT NOT NULL REFERENCES channel_identities(id) ON DELETE CASCADE,
  granted INTEGER NOT NULL DEFAULT 1,
  created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_channel_admins_unique UNIQUE (bot_id, channel_identity_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_channel_admins_bot_id ON bot_channel_admins(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_channel_admins_channel_identity_id ON bot_channel_admins(channel_identity_id);

-- user_channel_identity_bindings: global account-level binding between web users
-- and IM channel identities.
CREATE TABLE IF NOT EXISTS user_channel_identity_bindings (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  channel_identity_id TEXT NOT NULL REFERENCES channel_identities(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_channel_identity_bindings_unique UNIQUE (user_id, channel_identity_id)
);

CREATE INDEX IF NOT EXISTS idx_user_channel_identity_bindings_user_id ON user_channel_identity_bindings(user_id);
CREATE INDEX IF NOT EXISTS idx_user_channel_identity_bindings_channel_identity_id ON user_channel_identity_bindings(channel_identity_id);

-- channel_link_codes: one-time codes used to establish the binding from IM via
-- /link <code>.
CREATE TABLE IF NOT EXISTS channel_link_codes (
  token TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  channel_type TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  consumed_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_channel_link_codes_user_id ON channel_link_codes(user_id);
