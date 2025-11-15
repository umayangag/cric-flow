-- 0028_backfill_match_date.sql
-- One-time backfill for match_details.match_date to prevent NULL scan issues in calculators.
-- Strategy:
--  1) If legacy column "date" exists and is non-NULL, use it.
--  2) Otherwise, use a fallback derived from related activity; if nothing is available, use CURRENT_DATE.

BEGIN;

-- Step 1: Prefer legacy match_details.date when available
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'match_details' AND column_name = 'date'
  ) THEN
    EXECUTE 'UPDATE match_details
      SET match_date = date::date
      WHERE match_date IS NULL AND date IS NOT NULL';
  END IF;
END$$;

-- Step 2: Fallback using any activity joined to match_details (keeps it simple and safe)
-- Note: This uses the maximum of whatever non-null date we can find (prefers md2.date if present),
-- and finally CURRENT_DATE for rows that still remain NULL.
WITH fallback AS (
  SELECT md.match_id,
         COALESCE(MAX(md2.date), CURRENT_DATE) AS fallback_date
  FROM match_details md
  LEFT JOIN match_details md2 ON md2.match_id = md.match_id AND md2.date IS NOT NULL
  GROUP BY md.match_id
)
UPDATE match_details md
SET match_date = COALESCE(md.match_date, f.fallback_date, CURRENT_DATE)
FROM fallback f
WHERE md.match_id = f.match_id AND md.match_date IS NULL;

COMMIT;
