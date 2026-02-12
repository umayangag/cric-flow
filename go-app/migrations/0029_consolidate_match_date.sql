-- 0029_consolidate_match_date.sql
-- Consolidates 'date' and 'match_date' columns in match_details.
-- Simplified as there is no data in the database.

BEGIN;

DO $$
BEGIN
    -- If 'date' exists, rename it to 'match_date'
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='date') THEN
        ALTER TABLE match_details RENAME COLUMN date TO match_date;
    END IF;

    -- If 'match_date' still doesn't exist, add it
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        ALTER TABLE match_details ADD COLUMN match_date DATE;
    END IF;

    -- Ensure 'match_date' is NOT NULL
    ALTER TABLE match_details ALTER COLUMN match_date SET NOT NULL;
END $$;

-- Ensure index exists on match_date
CREATE INDEX IF NOT EXISTS idx_match_details_match_date ON match_details(match_date);

COMMIT;
