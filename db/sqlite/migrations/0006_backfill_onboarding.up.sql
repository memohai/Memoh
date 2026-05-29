-- Backfill existing users so they skip onboarding.
-- Tag backfilled rows with `onboarding_backfilled` so the down migration can roll
-- back ONLY what we wrote here, not real onboarding completions made afterward.
-- New users created after this migration have metadata='{}' (no onboarding_completed
-- key) and will be shown the onboarding flow.
UPDATE users
SET metadata = json_set(
  json_set(COALESCE(metadata, '{}'), '$.onboarding_completed', json('true')),
  '$.onboarding_backfilled', json('true')
)
WHERE json_extract(metadata, '$.onboarding_completed') IS NULL;
