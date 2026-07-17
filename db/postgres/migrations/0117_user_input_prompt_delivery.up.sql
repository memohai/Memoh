-- 0117_user_input_prompt_delivery
-- Record successful ask_user prompt delivery independently from reply binding.

ALTER TABLE user_input_requests
  ADD COLUMN IF NOT EXISTS prompt_delivered_at TIMESTAMPTZ;
