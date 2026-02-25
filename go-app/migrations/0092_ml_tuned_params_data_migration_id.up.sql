-- Link ml_tuned_params to data_migrations so we can derive training time and duration.
-- When auto_tune runs via the pipeline, tuned params are associated with the IN_PROGRESS
-- ml-auto-tune migration; after completion we can show trained_at (migration.started_at)
-- and duration (completed_at - started_at).
ALTER TABLE ml_tuned_params ADD COLUMN IF NOT EXISTS data_migration_id INT NULL REFERENCES data_migrations(id);
CREATE INDEX IF NOT EXISTS idx_ml_tuned_params_data_migration_id ON ml_tuned_params(data_migration_id) WHERE data_migration_id IS NOT NULL;
