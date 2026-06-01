-- 0009_command_ui_language
-- Add per-bot command-UI language (slash-command interface locale), independent
-- of `language` which controls the chat/agent reply language. 'auto' resolves to
-- the server default (English) at render time.
--
-- SQLite supports simple additive ADD COLUMN with a constant default, so no table
-- rebuild is required here.

ALTER TABLE bots ADD COLUMN command_ui_language TEXT NOT NULL DEFAULT 'auto';
