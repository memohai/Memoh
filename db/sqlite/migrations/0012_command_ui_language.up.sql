-- 0012_command_ui_language
-- Add per-bot command-UI language (slash-command interface locale), independent
-- of `language` which controls the chat/agent reply language. 'auto' resolves to
-- the server default (English) at render time.
--
-- Sequenced after 0010's full-table rebuild so this additive ADD COLUMN is not
-- silently dropped by a downstream CREATE TABLE that doesn't know about it.
-- Originally numbered 0009 on this branch; renumbered to 0012 when reconciling
-- with main's 0008 (bots.name) + 0010 (heartbeat default) which both rebuild
-- the bots table.

ALTER TABLE bots ADD COLUMN command_ui_language TEXT NOT NULL DEFAULT 'auto';
