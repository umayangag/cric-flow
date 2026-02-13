-- 0030_add_updated_at_to_match_details.sql
-- Adds updated_at column to match_details and a trigger to maintain it.

BEGIN;

-- 1) Add the column if it doesn't exist
ALTER TABLE match_details ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- 2) Create the trigger function if it doesn't exist
CREATE OR REPLACE FUNCTION public.set_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 3) Create the trigger
DROP TRIGGER IF EXISTS tr_match_details_set_updated_at ON match_details;
CREATE TRIGGER tr_match_details_set_updated_at
BEFORE UPDATE ON match_details
FOR EACH ROW EXECUTE FUNCTION public.set_updated_at_column();

COMMIT;
