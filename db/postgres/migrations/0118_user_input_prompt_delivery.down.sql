-- 0118_user_input_prompt_delivery rollback

ALTER TABLE user_input_requests
  DROP COLUMN IF EXISTS prompt_delivered_at;
