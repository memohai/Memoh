-- 0011_command_ui_language
-- Remove per-bot command-UI language.

ALTER TABLE bots DROP COLUMN command_ui_language;
