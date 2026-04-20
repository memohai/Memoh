-- 0067_add_image_model_type
-- Add image as a supported model type.
ALTER TABLE models DROP CONSTRAINT IF EXISTS models_type_check;

ALTER TABLE models
  ADD CONSTRAINT models_type_check
  CHECK (type IN ('chat', 'embedding', 'image', 'speech'));
