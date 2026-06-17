-- 0022_model_enable
-- Add a per-model enable flag so users can disable specific models within a
-- provider. Defaults to 1 (true) so existing rows stay listed in chat pickers.

ALTER TABLE models ADD COLUMN enable INTEGER NOT NULL DEFAULT 1;
