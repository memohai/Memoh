-- 0084_relax_reasoning_effort (down)
-- Restore the legacy low/medium/high CHECK constraint. Rows with newer effort
-- tiers (minimal/xhigh/max/none) must be reconciled before rolling back.

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'bots_reasoning_effort_check'
  ) THEN
    ALTER TABLE bots ADD CONSTRAINT bots_reasoning_effort_check
      CHECK (reasoning_effort IN ('low', 'medium', 'high'));
  END IF;
END $$;
