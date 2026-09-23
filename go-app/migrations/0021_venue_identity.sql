-- 0021_venue_identity.sql
--
-- Give `venue` the identity key it has carried a column and a unique index for since 0001
-- and never once had written to it (IMPORT-08).
--
-- `venue.normalized_name` and `ux_venue_normalized_name` were in the baseline. Nothing wrote
-- the column, so every row held NULL, the unique index constrained nothing (NULLs never
-- conflict in a btree), and a ground's identity was whatever string a match file happened to
-- spell. "M Chinnaswamy Stadium" and "M.Chinnaswamy Stadium" were two venues with two
-- familiarity histories and two scoring baselines.
--
-- The fold is `venues.NormalizeName` in Go and `venue_key` in ml-service's
-- ml/weather/venues.py, and it is deliberately conservative: accents dropped, case folded,
-- runs of punctuation and whitespace collapsed to one space, and nothing else. It does NOT
-- drop the part after a comma. "County Ground" names nine different grounds in this archive
-- -- bare, Bristol, Chelmsford, Derby, Hove, New Road, New Road/Worcester, Northampton and
-- Taunton -- and a rule keyed on the first comma-part would make them one. They stay nine.
-- Spellings that differ by more than punctuation, such as the four "Kensington Oval"
-- variants, also stay apart; folding those needs coordinates and is DATA-02's subject.
--
-- On the archive as it stands this merges exactly four pairs, 896 venue rows to 892 -- the
-- same 892 keys reference-data/venue-geocoding.csv already holds, which is what keeps that
-- table joinable:
--
--   Dr. Y.S. Rajasekhara Reddy ACA-VDCA Cricket Stadium  <- ... ACA VDCA ...
--   Gahanga International Cricket Stadium, Rwanda        <- ... Stadium. Rwanda
--   M Chinnaswamy Stadium                                <- M.Chinnaswamy Stadium
--   R Premadasa Stadium                                  <- R.Premadasa Stadium
--
-- The lowest id of each pair keeps the row, so the surviving `venue_name` is the spelling
-- the archive recorded first; the loser's matches, weather days and auction rows move onto
-- it and the loser is deleted.

BEGIN;

-- The backfill below folds in SQL what Go folds with NFKD decomposition, and the two agree
-- only while every stored name is ASCII -- as all 896 are. Refuse loudly rather than write a
-- key Go would disagree with: a silent disagreement here is the same class of defect this
-- migration exists to remove.
DO $$
DECLARE
    offending text;
BEGIN
    -- Under UTF-8 a name is pure ASCII exactly when its byte length equals its character
    -- length.
    SELECT venue_name INTO offending
    FROM public.venue
    WHERE octet_length(venue_name) <> char_length(venue_name)
    LIMIT 1;

    IF offending IS NOT NULL THEN
        RAISE EXCEPTION
            'venue_name % contains non-ASCII characters; fold it with venues.NormalizeName before migrating',
            quote_literal(offending);
    END IF;
END
$$;

-- Merge the rows that fold together, oldest id winning. This runs before the backfill
-- because ux_venue_normalized_name is a plain unique index: it is checked row by row, so a
-- backfill that produced two equal keys would fail mid-statement.
CREATE TEMPORARY TABLE venue_merge ON COMMIT DROP AS
WITH folded AS (
    SELECT id, btrim(regexp_replace(lower(venue_name), '[^a-z0-9]+', ' ', 'g')) AS venue_key
    FROM public.venue
),
keepers AS (
    SELECT venue_key, min(id) AS keeper_id FROM folded GROUP BY venue_key
)
SELECT folded.id AS loser_id, keepers.keeper_id, folded.venue_key
FROM folded
JOIN keepers ON keepers.venue_key = folded.venue_key
WHERE folded.id <> keepers.keeper_id;

UPDATE public.match m
SET venue_id = v.keeper_id
FROM venue_merge v
WHERE m.venue_id = v.loser_id;

-- venue_weather is keyed (venue_id, weather_date): drop a loser's day when the keeper
-- already holds that day, then move the rest. The readings are the same day at the same
-- ground either way, so the keeper's copy is not preferred over the loser's for any reason
-- beyond having to choose one.
DELETE FROM public.venue_weather w
USING venue_merge v
WHERE w.venue_id = v.loser_id
  AND EXISTS (
      SELECT 1 FROM public.venue_weather k
      WHERE k.venue_id = v.keeper_id AND k.weather_date = w.weather_date
  );

UPDATE public.venue_weather w
SET venue_id = v.keeper_id
FROM venue_merge v
WHERE w.venue_id = v.loser_id;

-- auction_venue is keyed (auction_id, venue_id): an auction that named both spellings of one
-- ground named one ground, so the duplicate row goes.
DELETE FROM public.auction_venue a
USING venue_merge v
WHERE a.venue_id = v.loser_id
  AND EXISTS (
      SELECT 1 FROM public.auction_venue k
      WHERE k.auction_id = a.auction_id AND k.venue_id = v.keeper_id
  );

UPDATE public.auction_venue a
SET venue_id = v.keeper_id
FROM venue_merge v
WHERE a.venue_id = v.loser_id;

DELETE FROM public.venue x
USING venue_merge v
WHERE x.id = v.loser_id;

UPDATE public.venue
SET normalized_name = btrim(regexp_replace(lower(venue_name), '[^a-z0-9]+', ' ', 'g'));

-- With every row keyed, the column becomes the thing a row cannot exist without. This is
-- what stops the next writer from reintroducing a venue with no identity.
ALTER TABLE public.venue
    ALTER COLUMN normalized_name SET NOT NULL;

COMMENT ON COLUMN public.venue.normalized_name IS
    'The ground''s identity: venue_name folded by venues.NormalizeName (Go) / venue_key (ml-service). Unique; the conflict target the importer upserts on and the key venue resolution matches (IMPORT-08)';
COMMENT ON COLUMN public.venue.venue_name IS
    'The spelling that first created this row, and the string /api/options/venues offers';
COMMENT ON COLUMN public.venue.city IS
    'The first city the archive named beside this ground, until the weather backfill overwrites it with the geocoded place; descriptive, no model reads it';
COMMENT ON COLUMN public.venue.display_name IS
    'Unused: nothing writes this and no query reads it. Venue resolution matches normalized_name, so a value here would not resolve (IMPORT-08)';

COMMIT;
