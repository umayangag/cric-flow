-- Add metrics JSONB column to store accuracy and other evaluation metrics from auto-tune.
ALTER TABLE ml_tuned_params ADD COLUMN IF NOT EXISTS metrics JSONB;
