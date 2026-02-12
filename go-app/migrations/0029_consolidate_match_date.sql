-- 0029_consolidate_match_date.sql
-- Renames 'date' to 'match_date' in match_details and merges data if necessary.

BEGIN;

-- 1. Ensure 'match_date' column exists; do NOT backfill per requirements.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        ALTER TABLE match_details ADD COLUMN match_date DATE;
    END IF;
END $$;

-- 2. (intentionally empty) Backfill skipped.

-- 3. Drop 'date' column if it still exists
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='date') THEN
        ALTER TABLE match_details DROP COLUMN date;
    END IF;
END $$;

-- 4. Ensure index exists on match_date
CREATE INDEX IF NOT EXISTS idx_match_details_match_date ON match_details(match_date);

COMMIT;
