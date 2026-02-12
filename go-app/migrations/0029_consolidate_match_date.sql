-- 0029_consolidate_match_date.sql
-- Consolidates 'date' and 'match_date' columns in match_details.
-- Simplified as there is no data in the database.

BEGIN;

-- Drop 'date' column if it exists
ALTER TABLE match_details DROP COLUMN IF EXISTS date;

-- Add 'match_date' column as NOT NULL if it doesn't exist
-- If it exists from a previous migration (like 0017), we just ensure it's NOT NULL
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        ALTER TABLE match_details ADD COLUMN match_date DATE;
    END IF;
END $$;

-- Ensure 'match_date' is NOT NULL
ALTER TABLE match_details ALTER COLUMN match_date SET NOT NULL;

-- Ensure index exists on match_date
CREATE INDEX IF NOT EXISTS idx_match_details_match_date ON match_details(match_date);

COMMIT;
