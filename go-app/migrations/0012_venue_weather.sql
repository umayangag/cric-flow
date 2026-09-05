-- 0012_venue_weather.sql
--
-- One reduced day of ERA5 weather per (venue, date) -- X-2 in docs/EXTERNAL_DATA_PLAN.md.
--
-- The archive says where and on what day a match was played, never what the air was like.
-- X-2 acquires that from the Open-Meteo ERA5 archive (CC BY 4.0, non-commercial use) and
-- keeps, per venue and match day, the day's 24 local hours of temperature, relative humidity
-- and precipitation, plus the precipitation totals of the seven days before -- what any
-- pre-match window (H-21) can be computed from, and nothing of the match's own hours that a
-- feature might read by mistake. The `venue` table already carried empty coordinate columns
-- (0001); the same backfill fills them from the curated geocoding table.
--
-- The durable copy of both is in git (reference-data/), not here: this table is rebuilt
-- offline by `make restore-venue-weather`, so a purge costs no network call. Nothing in the
-- rating pass reads it unless a weather family passes its gate; today none has.

BEGIN;

CREATE TABLE IF NOT EXISTS public.venue_weather (
    venue_id                    bigint      NOT NULL REFERENCES public.venue (id) ON DELETE CASCADE,
    weather_date                date        NOT NULL,
    timezone                    text        NOT NULL,
    hourly_temperature_c        real[]      NOT NULL,
    hourly_relative_humidity    real[]      NOT NULL,
    hourly_precipitation_mm     real[]      NOT NULL,
    prior_week_precipitation_mm real[]      NOT NULL,
    source                      text        NOT NULL,
    source_license              text        NOT NULL,
    fetched_at                  timestamptz,
    PRIMARY KEY (venue_id, weather_date)
);

COMMENT ON TABLE public.venue_weather IS
    'ERA5 readings for one venue on one match day, reduced (Open-Meteo archive, CC BY 4.0); restored from reference-data/';
COMMENT ON COLUMN public.venue_weather.hourly_temperature_c IS
    '24 values, local hours 00..23 of weather_date in the row''s timezone; null where the archive had none';
COMMENT ON COLUMN public.venue_weather.prior_week_precipitation_mm IS
    '7 daily totals for the seven days before weather_date, oldest first';

COMMIT;
