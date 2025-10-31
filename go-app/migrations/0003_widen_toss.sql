-- Migration: widen match_details.toss to accommodate full team names/phrases
-- This addresses errors like: value too long for type character varying(16)

DO $$
BEGIN
  -- Only widen when the column has a bounded length smaller than desired
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'match_details'
      AND column_name = 'toss'
      AND data_type = 'character varying'
      AND character_maximum_length IS NOT NULL
      AND character_maximum_length < 100
  ) THEN
    ALTER TABLE match_details
      ALTER COLUMN toss TYPE VARCHAR(100);
  END IF;
END $$;