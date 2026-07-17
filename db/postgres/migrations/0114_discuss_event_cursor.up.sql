-- 0114_discuss_event_cursor
-- Track event-cursor progress separately from the legacy source-time cursor.

ALTER TABLE bot_session_discuss_cursors
  ADD COLUMN IF NOT EXISTS consumed_event_cursor BIGINT NOT NULL DEFAULT 0;

ALTER POLICY teams_self_select ON public.teams USING (true);

DO $$
DECLARE
  migration_team_id UUID;
  previous_team_id TEXT;
BEGIN
  previous_team_id := current_setting('memoh.team_id', true);

  FOR migration_team_id IN SELECT id FROM public.teams ORDER BY id LOOP
    PERFORM set_config('memoh.team_id', migration_team_id::text, true);

    UPDATE bot_session_discuss_cursors AS c
    SET consumed_event_cursor = COALESCE((
      SELECT COALESCE(
        MAX(
          CASE
            WHEN e.event_data->>'event_cursor' ~ '^[1-9][0-9]{0,15}$' THEN
              CASE
                WHEN (e.event_data->>'event_cursor')::numeric <= 9007199254740991
                  THEN (e.event_data->>'event_cursor')::bigint
              END
          END
        ),
        MAX(e.received_at_ms)
      )
      FROM bot_session_events AS e
      WHERE e.team_id = migration_team_id
        AND e.session_id = c.session_id
        AND e.received_at_ms <= c.consumed_cursor
    ), 0)
    WHERE c.team_id = migration_team_id
      AND c.consumed_event_cursor = 0;
  END LOOP;

  PERFORM set_config('memoh.team_id', COALESCE(previous_team_id, ''), true);
END $$;

ALTER POLICY teams_self_select ON public.teams
  USING (id = public.memoh_current_team_id());
