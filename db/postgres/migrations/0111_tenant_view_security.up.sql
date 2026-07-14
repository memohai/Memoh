-- 0111_tenant_view_security
-- Fix the bot_visible_history_messages view so it cannot bypass tenant RLS.
--
-- New incremental (existing migrations untouched). The view was created without
-- security_invoker, so it executed with its (superuser) owner's privileges and
-- BYPASSED the FORCE RLS on bot_history_messages: a memoh_runtime connection
-- bound to tenant 2 could read tenant 1's messages through the view (SEV1
-- cross-tenant read). This migration:
--   1. recreates the view WITH (security_invoker = true) so it runs under the
--      caller's privileges — the base table's RLS then scopes it automatically;
--   2. projects tenant_id so consuming queries can carry explicit scope
--      (defense-in-depth) and so the schema guard can verify the view;
--   3. reassigns the view owner to memoh_owner (belt-and-suspenders: even if a
--      future change drops security_invoker, memoh_owner is NOLOGIN, non-super,
--      non-bypass, and is subject to FORCE RLS);
--   4. grants SELECT on the view to memoh_runtime (it was ungranted — the view
--      was also non-functional for the serving role).

-- Adding tenant_id as the first projected column changes column order, which
-- CREATE OR REPLACE VIEW rejects, so drop and recreate. No other object depends
-- on this view (verified), so a plain DROP is safe.
DROP VIEW IF EXISTS bot_visible_history_messages;

CREATE VIEW bot_visible_history_messages
WITH (security_invoker = true) AS
SELECT
  m.tenant_id,
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

ALTER VIEW bot_visible_history_messages OWNER TO memoh_owner;

GRANT SELECT ON bot_visible_history_messages TO memoh_runtime;
