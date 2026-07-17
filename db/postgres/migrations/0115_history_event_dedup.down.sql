-- 0115_history_event_dedup
-- Remove the event-to-history uniqueness boundary.

DROP INDEX IF EXISTS idx_bot_history_messages_event_id_unique;

ALTER POLICY teams_self_select ON public.teams USING (true);

DO $$
DECLARE
  migration_team_id UUID;
  previous_team_id TEXT;
BEGIN
  previous_team_id := current_setting('memoh.team_id', true);

  FOR migration_team_id IN SELECT id FROM public.teams ORDER BY id LOOP
    PERFORM set_config('memoh.team_id', migration_team_id::text, true);

    IF EXISTS (
      SELECT 1
      FROM bot_history_messages
      WHERE team_id = migration_team_id
        AND metadata ? '_migration_0115_history_event_dedup'
        AND NOT (
          event_id IS NULL
          AND jsonb_typeof(metadata->'_migration_0115_history_event_dedup') = 'object'
          AND metadata->'_migration_0115_history_event_dedup'->>'version' = '1'
          AND metadata->'_migration_0115_history_event_dedup'->>'message_id' = id::text
          AND metadata->'_migration_0115_history_event_dedup'->>'event_id'
            ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
        )
    ) THEN
      RAISE EXCEPTION 'invalid 0115 history event dedup rollback metadata';
    END IF;

    UPDATE bot_history_messages
    SET
      event_id = (metadata->'_migration_0115_history_event_dedup'->>'event_id')::uuid,
      metadata = metadata - '_migration_0115_history_event_dedup'
    WHERE team_id = migration_team_id
      AND metadata ? '_migration_0115_history_event_dedup';
  END LOOP;

  PERFORM set_config('memoh.team_id', COALESCE(previous_team_id, ''), true);
END $$;

ALTER POLICY teams_self_select ON public.teams
  USING (id = public.memoh_current_team_id());
