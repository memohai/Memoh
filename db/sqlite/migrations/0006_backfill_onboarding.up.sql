-- Backfill existing users so they skip onboarding.
-- New users created after this migration will have metadata='{}' (no onboarding_completed key)
-- and will be shown the onboarding flow.
UPDATE users
SET metadata = json_set(COALESCE(metadata, '{}'), '$.onboarding_completed', json('true'))
WHERE json_extract(metadata, '$.onboarding_completed') IS NULL;
