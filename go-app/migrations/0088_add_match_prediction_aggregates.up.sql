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
-- 1) Ensure the function exists (schema-qualified)
CREATE OR REPLACE FUNCTION public.mpa_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 2) Recreate trigger idempotently: drop if exists, then create
DROP TRIGGER IF EXISTS tr_mpa_set_updated_at ON match_prediction_aggregates;
CREATE TRIGGER tr_mpa_set_updated_at
BEFORE UPDATE ON match_prediction_aggregates
FOR EACH ROW EXECUTE FUNCTION public.mpa_set_updated_at();
