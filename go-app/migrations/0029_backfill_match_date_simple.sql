-- 0029_backfill_match_date_simple.sql
-- Simple one-time backfill to ensure match_details.match_date is non-NULL
BEGIN;
UPDATE match_details SET match_date = CURRENT_DATE WHERE match_date IS NULL;
COMMIT;
