-- 0030_remove_bot_members
-- Remove bot member sharing model and keep owner + guest ACL only.

DROP TABLE IF EXISTS bot_members;
