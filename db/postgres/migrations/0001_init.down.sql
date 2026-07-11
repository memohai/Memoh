ALTER TABLE IF EXISTS bot_channel_routes DROP CONSTRAINT IF EXISTS fk_bot_channel_routes_active_session;
ALTER TABLE IF EXISTS bot_history_messages DROP CONSTRAINT IF EXISTS fk_compact_id;

DROP TRIGGER IF EXISTS compaction_message_claim_finalize
  ON bot_history_message_compacts;
DROP TRIGGER IF EXISTS compaction_message_claim_insert_guard
  ON bot_history_messages;
DROP TRIGGER IF EXISTS compaction_message_claim_guard
  ON bot_history_messages;
DROP TRIGGER IF EXISTS compaction_log_terminal_artifact_guard
  ON bot_history_message_compacts;
DROP FUNCTION IF EXISTS finalize_compaction_message_claims();
DROP FUNCTION IF EXISTS guard_compaction_message_claim();
ALTER TABLE IF EXISTS bot_history_messages
  DROP CONSTRAINT IF EXISTS compact_claim_finalized_requires_owner,
  DROP COLUMN IF EXISTS compact_claim_finalized;

DROP TRIGGER IF EXISTS compaction_log_terminal_status_guard
  ON bot_history_message_compacts;
DROP FUNCTION IF EXISTS guard_compaction_log_terminal_status();

DROP TABLE IF EXISTS bot_user_grants CASCADE;
DROP TABLE IF EXISTS user_provider_oauth_tokens CASCADE;
DROP TABLE IF EXISTS provider_oauth_tokens CASCADE;
DROP TABLE IF EXISTS email_outbox CASCADE;
DROP TABLE IF EXISTS bot_email_bindings CASCADE;
DROP TABLE IF EXISTS email_oauth_tokens CASCADE;
DROP TABLE IF EXISTS email_providers CASCADE;
DROP TABLE IF EXISTS schedule_logs CASCADE;
DROP TABLE IF EXISTS bot_history_message_compact_parent_edges CASCADE;
DROP TABLE IF EXISTS bot_history_message_compacts CASCADE;
DROP FUNCTION IF EXISTS sync_compaction_artifact_parent_edges();
DROP TABLE IF EXISTS bot_heartbeat_logs CASCADE;
DROP TABLE IF EXISTS bot_history_message_assets CASCADE;
DROP TABLE IF EXISTS media_assets CASCADE;
DROP TABLE IF EXISTS bot_storage_bindings CASCADE;
DROP TABLE IF EXISTS storage_providers CASCADE;
DROP TABLE IF EXISTS schedule CASCADE;
DROP TABLE IF EXISTS lifecycle_events CASCADE;
DROP TABLE IF EXISTS container_versions CASCADE;
DROP TABLE IF EXISTS snapshots CASCADE;
DROP TABLE IF EXISTS bot_workspace_resource_limits CASCADE;
DROP TABLE IF EXISTS containers CASCADE;
DROP TABLE IF EXISTS user_input_requests CASCADE;
DROP TABLE IF EXISTS tool_approval_requests CASCADE;
DROP VIEW IF EXISTS bot_visible_history_messages CASCADE;
DROP TABLE IF EXISTS bot_history_messages CASCADE;
DROP TABLE IF EXISTS bot_session_events CASCADE;
DROP TABLE IF EXISTS bot_session_discuss_cursors CASCADE;
DROP TABLE IF EXISTS bot_sessions CASCADE;
DROP TABLE IF EXISTS bot_channel_routes CASCADE;
DROP TABLE IF EXISTS channel_identity_bind_codes CASCADE;
DROP TABLE IF EXISTS bot_channel_configs CASCADE;
DROP TABLE IF EXISTS mcp_oauth_tokens CASCADE;
DROP TABLE IF EXISTS bot_plugin_resources CASCADE;
DROP TABLE IF EXISTS mcp_connections CASCADE;
DROP TABLE IF EXISTS bot_plugin_installations CASCADE;
DROP TABLE IF EXISTS channel_link_codes CASCADE;
DROP TABLE IF EXISTS user_channel_identity_bindings CASCADE;
DROP TABLE IF EXISTS bot_channel_admins CASCADE;
DROP TABLE IF EXISTS bot_acl_rules CASCADE;
DROP TABLE IF EXISTS tasks CASCADE;
DROP TABLE IF EXISTS bot_inbox CASCADE;
DROP TABLE IF EXISTS subagents CASCADE;
DROP TABLE IF EXISTS bot_preauth_keys CASCADE;
DROP TABLE IF EXISTS bot_members CASCADE;
DROP TABLE IF EXISTS bots CASCADE;
DROP TABLE IF EXISTS browser_contexts CASCADE;
DROP TABLE IF EXISTS tts_models CASCADE;
DROP TABLE IF EXISTS tts_providers CASCADE;
DROP TABLE IF EXISTS memory_providers CASCADE;
DROP TABLE IF EXISTS model_variants CASCADE;
DROP TABLE IF EXISTS models CASCADE;
DROP TABLE IF EXISTS llm_provider_oauth_tokens CASCADE;
DROP TABLE IF EXISTS llm_providers CASCADE;
DROP TABLE IF EXISTS fetch_providers CASCADE;
DROP TABLE IF EXISTS search_providers CASCADE;
DROP TABLE IF EXISTS providers CASCADE;
DROP TABLE IF EXISTS user_channel_bindings CASCADE;
DROP TABLE IF EXISTS channel_identities CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TYPE IF EXISTS user_role;
