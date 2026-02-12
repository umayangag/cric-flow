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
    
    -- 2. If both exist, we drop 'date' (as per previous instructions to not backfill, 
    -- but user now wants to preserve data via rename, so this case assumes 'match_date' already has data or is the intended target)
    ELSIF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='date') AND
          EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        ALTER TABLE match_details DROP COLUMN date;
    
    -- 3. If neither exists (unlikely), add 'match_date'
    ELSIF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='match_details' AND column_name='match_date') THEN
        ALTER TABLE match_details ADD COLUMN match_date DATE;
    END IF;
END $$;

-- 4. Ensure index exists on match_date
CREATE INDEX IF NOT EXISTS idx_match_details_match_date ON match_details(match_date);

COMMIT;
