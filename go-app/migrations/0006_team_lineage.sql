-- 0006_team_lineage.sql
--
-- A club that renames is one club, not two (I-4 in docs/IDENTITY_PR_CHECKLIST.md).
--
-- Cricsheet names a team by whatever it was called on the day, so a rebrand splits a club
-- into two opposition rows and its history restarts at the boundary:
--
--    id  |       opposition_name       | matches | first_match | last_match
--    292 | Royal Challengers Bangalore |     240 | 2008-04-18  | 2024-03-17
--   1038 | Royal Challengers Bengaluru |      46 | 2024-03-22  | 2026-05-31
--
-- Every team-level feature the win model reads -- Elo, recent form, head to head, venue
-- familiarity -- is accumulated per opposition id, so on the day of a rename a club with
-- sixteen years of history becomes a debutant.
--
-- canonical_id points a superseded row at the club's current row, and is null for a club
-- that has never renamed, so "the club" is COALESCE(canonical_id, id). The display name
-- stays per row: a 2019 scorecard still reads "Delhi Daredevils", because that is who
-- played.
--
-- The mapping itself is *data*, in configs/team_lineage.json, reviewed by hand. It is not
-- applied here. Opposition ids are assigned by the importer and are not stable across a
-- rebuild, so a migration that hard-coded them would be wrong the first time anyone
-- re-imports; the importer resolves the names and writes this column at the end of a run.

BEGIN;

ALTER TABLE public.opposition
    ADD COLUMN canonical_id bigint;

ALTER TABLE public.opposition
    ADD CONSTRAINT opposition_canonical_id_fkey FOREIGN KEY (canonical_id) REFERENCES public.opposition(id);

-- A row may not be its own successor: that is the null case, and storing it as a
-- self-reference would make "has this club renamed?" two questions instead of one.
ALTER TABLE public.opposition
    ADD CONSTRAINT opposition_canonical_id_not_self CHECK (canonical_id IS NULL OR canonical_id <> id);

-- Resolving a club reads this column for one id at a time, and the pool query reads it the
-- other way round -- every row belonging to one club.
CREATE INDEX IF NOT EXISTS opposition_canonical_id_idx
    ON public.opposition (canonical_id) WHERE canonical_id IS NOT NULL;

COMMIT;
