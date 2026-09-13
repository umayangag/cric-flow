-- 0018_ball_event_extras_breakdown.sql
--
-- A delivery's extras by kind (IMPORT-04 in docs/AUDIT_FINDINGS.md).
--
-- Cricsheet records a delivery's extras as an object of up to five counts -- wides,
-- noballs, byes, legbyes, penalty -- and a delivery can carry two of them: a no-ball with
-- leg-byes off it, a penalty beside a wide. ball_event kept their sum (runs_extras) and
-- one name chosen by precedence (extras_kind), so a no-ball with four leg-byes was stored
-- as extras_kind = 'no_ball', runs_extras = 5, and nothing in the row could say that
-- four of the five were leg-byes. That mattered because byes, leg-byes and penalty runs
-- are not the bowler's: the importer charged him the delivery's whole total, and the
-- rating pass sums runs_total for runs conceded (FEAT-08), so every bowler's runs carried
-- his keeper's misses.
--
-- Five columns rather than one derived runs_bowler, because the counts are the facts and
-- the derivation is one question of several. runs_extras is their sum on every delivery
-- in the archive; extras_kind stays as the summary it always was, now recoverable from
-- the row that carries it. smallint holds the largest value the archive has (a penalty
-- of 12) with room to spare.
--
-- NOT NULL DEFAULT 0 is a metadata-only change on a table this size, and a zero is the
-- truth for a delivery with no extras. It is not the truth for a row imported before
-- this migration: those keep zeros in all five until the directory is re-imported, which
-- is what writes them.

BEGIN;

ALTER TABLE public.ball_event
    ADD COLUMN IF NOT EXISTS extras_wides smallint DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS extras_noballs smallint DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS extras_byes smallint DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS extras_legbyes smallint DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS extras_penalty smallint DEFAULT 0 NOT NULL;

COMMENT ON COLUMN public.ball_event.extras_wides IS
    'Cricsheet extras.wides on this delivery; charged to the bowler';
COMMENT ON COLUMN public.ball_event.extras_noballs IS
    'Cricsheet extras.noballs on this delivery; charged to the bowler';
COMMENT ON COLUMN public.ball_event.extras_byes IS
    'Cricsheet extras.byes on this delivery; not charged to the bowler';
COMMENT ON COLUMN public.ball_event.extras_legbyes IS
    'Cricsheet extras.legbyes on this delivery; not charged to the bowler';
COMMENT ON COLUMN public.ball_event.extras_penalty IS
    'Cricsheet extras.penalty on this delivery; not charged to the bowler';
COMMENT ON COLUMN public.ball_event.extras_kind IS
    'One kind of extra by precedence (wide, no_ball, leg_bye, bye, penalty): a lossy summary of the extras_* columns';

COMMIT;
