-- 0115_history_event_dedup
-- Enforce one durable history linkage per event for replay deduplication.

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
      RAISE EXCEPTION 'reserved 0115 history event dedup metadata marker is already in use';
    END IF;

    WITH ranked_event_links AS (
      SELECT
        id,
        event_id,
        ROW_NUMBER() OVER (
          PARTITION BY event_id
          ORDER BY created_at, id
        ) AS link_number
      FROM bot_history_messages
      WHERE team_id = migration_team_id
        AND event_id IS NOT NULL
    )
    UPDATE bot_history_messages AS message
    SET
      metadata = jsonb_set(
        message.metadata,
        '{_migration_0115_history_event_dedup}',
        jsonb_build_object(
          'version', 1,
          'message_id', message.id::text,
          'event_id', ranked.event_id::text
        ),
        true
      ),
      event_id = NULL
    FROM ranked_event_links AS ranked
    WHERE message.team_id = migration_team_id
      AND message.id = ranked.id
      AND ranked.link_number > 1;
  END LOOP;

  PERFORM set_config('memoh.team_id', COALESCE(previous_team_id, ''), true);
END $$;

ALTER POLICY teams_self_select ON public.teams
  USING (id = public.memoh_current_team_id());

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_event_id_unique
  ON bot_history_messages(event_id)
  WHERE event_id IS NOT NULL;
