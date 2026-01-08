-- Drop trigger and function if exist (schema-qualified)
DROP TRIGGER IF EXISTS tr_mpa_set_updated_at ON match_prediction_aggregates;
DROP FUNCTION IF EXISTS public.mpa_set_updated_at();

-- Drop indexes and table
DROP INDEX IF EXISTS idx_mpa_teams_date;
DROP INDEX IF EXISTS idx_mpa_format_date;
DROP TABLE IF EXISTS match_prediction_aggregates;
