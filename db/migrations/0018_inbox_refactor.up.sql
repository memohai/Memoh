-- 0018_inbox_refactor
-- Refactor bot_inbox: split content JSONB into content TEXT + header JSONB, add action column.

-- 1. Add new columns.
ALTER TABLE bot_inbox ADD COLUMN IF NOT EXISTS header JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE bot_inbox ADD COLUMN IF NOT EXISTS action TEXT NOT NULL DEFAULT 'notify';

-- 2. Migrate existing data: extract "text" into header, keep remaining keys.
UPDATE bot_inbox
SET header  = content - 'text',
    action  = 'notify'
WHERE content IS NOT NULL AND content::text <> '{}';

-- 3. Convert content column from JSONB to TEXT.
--    Extract the "text" key as the new plain-text content.
ALTER TABLE bot_inbox ALTER COLUMN content DROP DEFAULT;
ALTER TABLE bot_inbox ALTER COLUMN content TYPE TEXT USING COALESCE(content ->> 'text', '');
ALTER TABLE bot_inbox ALTER COLUMN content SET DEFAULT '';
