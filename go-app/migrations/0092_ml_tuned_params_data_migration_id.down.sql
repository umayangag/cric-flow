DROP INDEX IF EXISTS idx_ml_tuned_params_data_migration_id;
ALTER TABLE ml_tuned_params DROP COLUMN IF EXISTS data_migration_id;
