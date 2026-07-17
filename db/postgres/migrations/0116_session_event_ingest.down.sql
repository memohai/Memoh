-- 0116_session_event_ingest
-- Remove event-delivery lease columns and the durable event cursor sequence.

ALTER TABLE bot_session_events
  DROP COLUMN IF EXISTS delivery_completed_at,
  DROP COLUMN IF EXISTS delivery_claimed_until,
  DROP COLUMN IF EXISTS delivery_claim_token;

DROP SEQUENCE IF EXISTS bot_session_event_cursor_seq;
