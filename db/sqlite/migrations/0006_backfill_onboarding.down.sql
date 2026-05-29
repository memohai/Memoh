-- Only roll back rows this migration backfilled, never real user completions.
UPDATE users
SET metadata = json_remove(metadata, '$.onboarding_completed', '$.onboarding_backfilled')
WHERE json_extract(metadata, '$.onboarding_backfilled') IS NOT NULL;
