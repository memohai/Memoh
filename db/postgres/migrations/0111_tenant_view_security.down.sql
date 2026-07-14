-- 0111_tenant_view_security (down)
-- Restore the view to its pre-0111 shape (the 0103 definition: no tenant_id
-- projection, no security_invoker). This reverts to the prior state; the
-- security fix lives only in the up.

-- Column order changes (tenant_id-first back to turn_id-first), so drop+recreate.
DROP VIEW IF EXISTS bot_visible_history_messages;

CREATE VIEW bot_visible_history_messages AS
SELECT
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id,
  m.source_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.compact_id,
  m.session_mode,
  m.runtime_type,
  m.model_id,
  m.event_id,
  m.display_text,
  m.created_at
FROM bot_history_messages m
WHERE m.turn_visible = true
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL;
