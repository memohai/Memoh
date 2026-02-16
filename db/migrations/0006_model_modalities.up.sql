-- Replace is_multimodal boolean with input modality array.
ALTER TABLE models ADD COLUMN IF NOT EXISTS input_modalities TEXT[] NOT NULL DEFAULT ARRAY['text']::TEXT[];

-- Migrate existing data: true -> ['text','image'], false -> ['text']
UPDATE models SET input_modalities = ARRAY['text','image']::TEXT[] WHERE is_multimodal = true;
UPDATE models SET input_modalities = ARRAY['text']::TEXT[] WHERE is_multimodal = false;

ALTER TABLE models DROP COLUMN IF EXISTS is_multimodal;
