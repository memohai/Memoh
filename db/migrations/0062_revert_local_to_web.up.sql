-- 0062_revert_local_to_web
-- Revert channel_type 'local' back to 'web' to match updated adapter constants.
-- The original 0056 migration merged web/cli → local; this undoes that change.

UPDATE channel_identities SET channel_type = 'web' WHERE channel_type = 'local';
UPDATE user_channel_bindings SET channel_type = 'web' WHERE channel_type = 'local';
UPDATE bot_channel_configs SET channel_type = 'web' WHERE channel_type = 'local';
UPDATE channel_identity_bind_codes SET channel_type = 'web' WHERE channel_type = 'local';
UPDATE bot_channel_routes SET channel_type = 'web' WHERE channel_type = 'local';
UPDATE bot_sessions SET channel_type = 'web' WHERE channel_type = 'local';
