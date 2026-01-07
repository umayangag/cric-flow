-- Drop trigger and function if exist
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'tr_mpa_set_updated_at'
    ) THEN
        DROP TRIGGER tr_mpa_set_updated_at ON match_prediction_aggregates;
    END IF;
    IF EXISTS (
        SELECT 1 FROM pg_proc WHERE proname = 'mpa_set_updated_at'
    ) THEN
        DROP FUNCTION mpa_set_updated_at();
    END IF;
END$$;

-- Drop indexes and table
DROP INDEX IF EXISTS idx_mpa_teams_date;
DROP INDEX IF EXISTS idx_mpa_format_date;
DROP TABLE IF EXISTS match_prediction_aggregates;
