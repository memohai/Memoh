-- 0004_remove_channel_identity_binding (rollback)
-- Recreate channel identity user linking and one-time bind codes.

ALTER TABLE channel_identities
  ADD COLUMN user_id TEXT REFERENCES users(id);

CREATE INDEX IF NOT EXISTS idx_channel_identities_user_id ON channel_identities(user_id);

CREATE TABLE IF NOT EXISTS channel_identity_bind_codes (
  id TEXT PRIMARY KEY,
  token TEXT NOT NULL,
  issued_by_user_id TEXT NOT NULL REFERENCES users(id),
  channel_type TEXT,
  expires_at TEXT,
  used_at TEXT,
  used_by_channel_identity_id TEXT REFERENCES channel_identities(id),
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT channel_identity_bind_codes_token_unique UNIQUE (token)
);

CREATE INDEX IF NOT EXISTS idx_channel_identity_bind_codes_channel_type ON channel_identity_bind_codes(channel_type);
