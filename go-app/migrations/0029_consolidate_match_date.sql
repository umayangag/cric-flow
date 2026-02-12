-- 0029_consolidate_match_date.sql
-- Consolidates 'date' and 'match_date' columns in match_details.
-- Renames 'date' to 'match_date' to preserve data if 'match_date' does not exist.

BEGIN;

DO $$
BEGIN
    -- 1. If 'date' exists and 'match_date' does not, rename 'date' to 'match_date'
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='date') AND
       NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        ALTER TABLE match_details RENAME COLUMN date TO match_date;
    
    -- 2. If both exist, copy data from 'date' to 'match_date' where 'match_date' is NULL to prevent data loss, then drop 'date'.
    ELSIF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='date') AND
          EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        UPDATE match_details SET match_date = date WHERE match_date IS NULL;
        ALTER TABLE match_details DROP COLUMN date;
    
    -- 3. If neither exists (unlikely), add 'match_date'
    ELSIF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        ALTER TABLE match_details ADD COLUMN match_date DATE;
    END IF;

    -- 4. Make match_date NOT NULL.
    -- To ensure we can set NOT NULL, we check for remaining NULLs.
    -- If any exist, we fail the migration to ensure data quality.
    IF EXISTS (SELECT 1 FROM match_details WHERE match_date IS NULL) THEN
        RAISE EXCEPTION 'Found NULL values in match_date. Data quality check failed.';
    END IF;

    ALTER TABLE match_details ALTER COLUMN match_date SET NOT NULL;
END $$;

-- 5. Ensure index exists on match_date
CREATE INDEX IF NOT EXISTS idx_match_details_match_date ON match_details(match_date);

COMMIT;
