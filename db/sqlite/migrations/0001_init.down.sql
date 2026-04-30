-- 0001_init
-- Drop SQLite initial schema.

PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS user_provider_oauth_tokens;
DROP TABLE IF EXISTS provider_oauth_tokens;
DROP TABLE IF EXISTS email_outbox;
DROP TABLE IF EXISTS bot_email_bindings;
DROP TABLE IF EXISTS email_oauth_tokens;
DROP TABLE IF EXISTS email_providers;
DROP TABLE IF EXISTS schedule_logs;
DROP TABLE IF EXISTS bot_history_message_compacts;
DROP TABLE IF EXISTS bot_heartbeat_logs;
DROP TABLE IF EXISTS bot_history_message_assets;
DROP TABLE IF EXISTS bot_storage_bindings;
DROP TABLE IF EXISTS storage_providers;
DROP TABLE IF EXISTS schedule;
DROP TABLE IF EXISTS lifecycle_events;
DROP TABLE IF EXISTS container_versions;
DROP TABLE IF EXISTS snapshots;
DROP TABLE IF EXISTS containers;
DROP TABLE IF EXISTS tool_approval_requests;
DROP TABLE IF EXISTS bot_history_messages;
DROP TABLE IF EXISTS bot_session_events;
DROP TABLE IF EXISTS bot_sessions;
DROP TABLE IF EXISTS bot_channel_routes;
DROP TABLE IF EXISTS channel_identity_bind_codes;
DROP TABLE IF EXISTS bot_channel_configs;
DROP TABLE IF EXISTS mcp_oauth_tokens;
DROP TABLE IF EXISTS mcp_connections;
DROP TABLE IF EXISTS bot_acl_rules;
DROP TABLE IF EXISTS bots;
DROP TABLE IF EXISTS browser_contexts;
DROP TABLE IF EXISTS memory_providers;
DROP TABLE IF EXISTS model_variants;
DROP TABLE IF EXISTS models;
DROP TABLE IF EXISTS search_providers;
DROP TABLE IF EXISTS providers;
DROP TABLE IF EXISTS user_channel_bindings;
DROP TABLE IF EXISTS channel_identities;
DROP TABLE IF EXISTS users;

PRAGMA foreign_keys = ON;
