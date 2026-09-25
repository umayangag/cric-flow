-- 0025_venue_name_unique_dropped.sql
--
-- Drop `venue_venue_name_key`, the unique constraint the venue upsert cannot arbitrate
-- (B-23 in docs/BUG_BACKLOG.md).
--
-- `venue` carried two unique keys: `venue_venue_name_key UNIQUE (venue_name)` from the
-- 0001 baseline, and `ux_venue_normalized_name UNIQUE (normalized_name)`, which IMPORT-08
-- (0021) filled and made the ground's identity. `db.GetOrCreateVenue` upserts
-- `ON CONFLICT (normalized_name)` -- and ON CONFLICT arbitrates only the index it names.
-- A conflict found while writing any *other* unique index is not absorbed into the DO
-- UPDATE; it is raised as `duplicate key value violates unique constraint
-- "venue_venue_name_key" (SQLSTATE 23505)` and the match file fails.
--
-- Sequentially the two upserts both succeed, because the first one's row is already
-- committed when the second one pre-checks the arbiter. The failure is a race: when two
-- importer goroutines first name one ground at the same instant, both pre-check the
-- arbiter, both find nothing, and both go on to write their index entries --
-- `venue_venue_name_key` first, since it sorts before the arbiter by oid. The loser gets
-- the 23505. A full import of the 22,905-file archive into an empty database lost seven
-- files this way (1094702, 1269073, 1331386, 1393877, 1446104, 1548659, 584924), and
-- because `-fail-fast` defaults to true the same run under `make import` stopped at file
-- ~4,025. 66 of the archive's 892 grounds are spelled identically by files that place them
-- in different cities, and the importer's cache keys on ground plus city, so those are the
-- lookups that reach the database more than once and can collide.
--
-- 0004 did this correctly for the other two dimensions when it moved their conflict
-- targets: `player_player_name_key` and `opposition_opposition_name_key` were dropped in
-- the same migration that introduced `player_external_id_key` and
-- `opposition_name_gender_key`. 0021 moved the venue target and left the old key standing.
-- This finishes 0021.
--
-- Nothing is given up by dropping it. `normalized_name` is `venue_name` folded by
-- `venues.NormalizeName` and is written nowhere else, so two rows with the same
-- `venue_name` would necessarily have the same `normalized_name` and are already refused
-- by the arbiter: uniqueness of the spelling is implied by uniqueness of the identity, and
-- the implication is the strict one -- a key the importer can absorb replaces one it
-- cannot. No foreign key references `venue_name`, and no query looks a venue up by it:
-- `/api/options/venues` lists `SELECT DISTINCT venue_name ... ILIKE`, which a btree
-- equality index could not serve anyway, and venue resolution matches `normalized_name`
-- (IMPORT-08).

BEGIN;

ALTER TABLE public.venue DROP CONSTRAINT IF EXISTS venue_venue_name_key;

COMMENT ON COLUMN public.venue.venue_name IS
    'The spelling that first created this row, and the string /api/options/venues offers. Not unique in its own right: normalized_name is the key, and it is the only one the importer''s ON CONFLICT can arbitrate (B-23)';

COMMIT;
