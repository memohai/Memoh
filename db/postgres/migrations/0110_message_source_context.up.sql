-- 0110_message_source_context
-- Add dormant storage for the versioned message-owned source envelope.

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS source_context JSONB;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'history_message_source_context_valid'
      AND conrelid = 'bot_history_messages'::regclass
  ) THEN
    ALTER TABLE bot_history_messages
      ADD CONSTRAINT history_message_source_context_valid
      CHECK (
        source_context IS NULL
        OR (
          jsonb_typeof(source_context) = 'object'
          AND source_context ?& ARRAY[
            'version',
            'sender_display_name',
            'platform',
            'conversation_type',
            'conversation_name'
          ]
          AND source_context - ARRAY[
            'version',
            'sender_display_name',
            'platform',
            'conversation_type',
            'conversation_name'
          ] = '{}'::jsonb
          AND jsonb_typeof(source_context->'version') = 'number'
          AND source_context->'version' = '1'::jsonb
          AND source_context->>'version' = '1'
          AND jsonb_typeof(source_context->'sender_display_name') = 'string'
          AND jsonb_typeof(source_context->'platform') = 'string'
          AND jsonb_typeof(source_context->'conversation_type') = 'string'
          AND jsonb_typeof(source_context->'conversation_name') = 'string'
        )
      ) NOT VALID;
  END IF;
END
$$;
