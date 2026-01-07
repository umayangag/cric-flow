-- Create table for caching match-level prediction aggregates
CREATE TABLE IF NOT EXISTS match_prediction_aggregates (
    match_id BIGINT PRIMARY KEY,
    format TEXT NOT NULL,
    team1_code TEXT NOT NULL,
    team2_code TEXT NOT NULL,
    predicted_winner_code TEXT NULL,
    predicted_total_runs DOUBLE PRECISION NULL,
    model_version TEXT NULL,
    cutoff_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mpa_format_date ON match_prediction_aggregates (format, cutoff_at);
CREATE INDEX IF NOT EXISTS idx_mpa_teams_date ON match_prediction_aggregates (team1_code, team2_code, cutoff_at);

-- Trigger to auto-update updated_at on UPDATE
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_proc WHERE proname = 'mpa_set_updated_at'
    ) THEN
        CREATE OR REPLACE FUNCTION mpa_set_updated_at()
        RETURNS TRIGGER AS $$
        BEGIN
            NEW.updated_at := now();
            RETURN NEW;
        END;
        $$ LANGUAGE plpgsql;
    END IF;
END$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'tr_mpa_set_updated_at'
    ) THEN
        CREATE TRIGGER tr_mpa_set_updated_at
        BEFORE UPDATE ON match_prediction_aggregates
        FOR EACH ROW EXECUTE FUNCTION mpa_set_updated_at();
    END IF;
END$$;
