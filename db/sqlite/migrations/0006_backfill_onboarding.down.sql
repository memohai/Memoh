UPDATE users
SET metadata = json_remove(metadata, '$.onboarding_completed');
