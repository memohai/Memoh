-- 0117_session_event_ingest
-- Allocate process-independent unique cursors and lease same-event delivery across instances.
-- Cursor values do not serialize distinct same-session events across processes.

CREATE SEQUENCE IF NOT EXISTS bot_session_event_cursor_seq
  AS BIGINT
  MINVALUE 1
  MAXVALUE 9007199254740991;

ALTER POLICY teams_self_select ON public.teams USING (true);

DO $$
DECLARE
  migration_team_id UUID;
  previous_team_id TEXT;
  legacy_floor BIGINT := 1;
  team_floor BIGINT;
  current_floor BIGINT;
  clock_floor BIGINT;
  cursor_floor BIGINT;
BEGIN
  previous_team_id := current_setting('memoh.team_id', true);

  FOR migration_team_id IN SELECT id FROM public.teams ORDER BY id LOOP
    PERFORM set_config('memoh.team_id', migration_team_id::text, true);

    SELECT COALESCE(MAX(
      CASE
        WHEN event_data->>'event_cursor' ~ '^[1-9][0-9]{0,15}$'
          THEN LEAST((event_data->>'event_cursor')::numeric, 9007199254740991)::bigint
        ELSE GREATEST(received_at_ms, 1)
      END
    ), 1)
    INTO team_floor
    FROM bot_session_events
    WHERE team_id = migration_team_id;

    legacy_floor := GREATEST(legacy_floor, team_floor);
  END LOOP;

  SELECT last_value INTO current_floor FROM bot_session_event_cursor_seq;
  clock_floor := FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::bigint;
  cursor_floor := GREATEST(legacy_floor, current_floor, clock_floor);
  IF cursor_floor >= 9007199254740991 THEN
    RAISE EXCEPTION 'session event cursor exhausted JSON-safe integer range';
  END IF;
  PERFORM setval('bot_session_event_cursor_seq', cursor_floor, true);
  PERFORM set_config('memoh.team_id', COALESCE(previous_team_id, ''), true);
END $$;

ALTER POLICY teams_self_select ON public.teams
  USING (id = public.memoh_current_team_id());

ALTER TABLE bot_session_events
  ADD COLUMN IF NOT EXISTS delivery_claim_token UUID,
  ADD COLUMN IF NOT EXISTS delivery_claimed_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS delivery_completed_at TIMESTAMPTZ;
