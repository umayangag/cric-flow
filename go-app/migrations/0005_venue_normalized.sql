-- 0005_venue_normalized.sql
-- Extend existing venue table to store normalized and geocoded information.

ALTER TABLE IF EXISTS venue
ADD COLUMN IF NOT EXISTS normalized_name TEXT,
ADD COLUMN IF NOT EXISTS display_name TEXT,
ADD COLUMN IF NOT EXISTS city TEXT,
ADD COLUMN IF NOT EXISTS country TEXT,
ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION,
ADD COLUMN IF NOT EXISTS timezone TEXT,
ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'open-meteo-geocoding',
ADD COLUMN IF NOT EXISTS confidence REAL,
ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Backfill display_name from venue_name if empty
UPDATE venue SET display_name = COALESCE(display_name, venue_name);

-- Backfill normalized_name as a lowercase trimmed version if empty
UPDATE venue SET normalized_name = COALESCE(normalized_name, regexp_replace(lower(trim(venue_name)), '\\s+', ' ', 'g'));

-- Unique index on normalized_name for lookups
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'ux_venue_normalized_name'
    ) THEN
        CREATE UNIQUE INDEX ux_venue_normalized_name ON venue(normalized_name);
    END IF;
END $$;
