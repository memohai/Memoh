-- 0107_rls_enforcement (down)
-- Reverse FORCE ROW LEVEL SECURITY on every tenant table and remove the
-- memoh_app runtime role together with its grants and default privileges.

DO $$
DECLARE t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'team_members','channel_identities','user_channel_bindings','providers',
    'search_providers','fetch_providers','models','model_variants','memory_providers',
    'bots','bot_acl_rules','bot_channel_admins','user_channel_identity_bindings',
    'channel_link_codes','bot_plugin_installations','mcp_connections','bot_plugin_resources',
    'mcp_oauth_tokens','bot_channel_configs','channel_identity_bind_codes','bot_channel_routes',
    'bot_sessions','bot_session_events','bot_history_messages','bot_session_discuss_cursors',
    'tool_approval_requests','user_input_requests','containers','bot_workspace_resource_limits',
    'snapshots','container_versions','lifecycle_events','schedule','bot_storage_bindings',
    'media_assets','bot_history_message_assets','bot_heartbeat_logs','bot_history_message_compacts',
    'schedule_logs','email_providers','email_oauth_tokens','bot_email_bindings','email_outbox',
    'provider_oauth_tokens','user_provider_oauth_tokens','bot_user_grants','memory_nodes',
    'memory_edges','browser_contexts','tasks'
  ]
  LOOP
    IF to_regclass('public.'||quote_ident(t)) IS NOT NULL THEN
      EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
    END IF;
  END LOOP;
END $$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'memoh_app') THEN
    ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM memoh_app;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE USAGE, SELECT ON SEQUENCES FROM memoh_app;
    REVOKE ALL ON ALL TABLES IN SCHEMA public FROM memoh_app;
    REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM memoh_app;
    REVOKE USAGE ON SCHEMA public FROM memoh_app;
    DROP ROLE IF EXISTS memoh_app;
  END IF;
END $$;
