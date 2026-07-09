-- 0107_rls_enforcement
-- Make the team_isolation policy actually enforce: FORCE ROW LEVEL SECURITY on
-- every tenant table and connect the runtime as a non-owner, non-superuser role
-- (memoh_app) so the policy is no longer bypassed. The FORCE table list below is
-- kept verbatim in sync with the ENABLE ROW LEVEL SECURITY list in
-- 0105_team_multitenancy.up.sql.

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
      EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    END IF;
  END LOOP;
END $$;

-- Runtime application role: LOGIN, NOT superuser, NOT table owner, so FORCE RLS
-- applies to it. Deployments should reset the password after the migration runs
-- (ALTER ROLE memoh_app PASSWORD :'pw' via entrypoint, or an env-driven value).
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'memoh_app') THEN
    CREATE ROLE memoh_app LOGIN PASSWORD 'memoh_app';
  END IF;
END $$;

GRANT USAGE ON SCHEMA public TO memoh_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO memoh_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO memoh_app;
-- Future tables/sequences created by the owner role:
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO memoh_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO memoh_app;
