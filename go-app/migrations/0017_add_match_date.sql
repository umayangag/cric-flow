-- 0017_add_match_date.sql
-- Adds match_date to match_details for latest-as-of computations.

BEGIN;

ALTER TABLE match_details
    ADD COLUMN IF NOT EXISTS match_date DATE;

-- Optional: backfill could be done later via a dedicated script/command.
-- Create an index to accelerate joins by match date.
CREATE INDEX IF NOT EXISTS idx_match_details_match_date ON match_details(match_date);

COMMIT;
