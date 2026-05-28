-- Backfill existing users so they skip onboarding.
-- New users created after this migration will have metadata='{}' (no onboarding_completed key)
-- and will be shown the onboarding flow.
UPDATE users
SET metadata = jsonb_set(COALESCE(metadata, '{}'), '{onboarding_completed}', 'true')
WHERE metadata->>'onboarding_completed' IS NULL;
