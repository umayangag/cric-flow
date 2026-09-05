-- 0011_match_event_stage.sql
--
-- The two event fields the importer was dropping (X-3 in docs/EXTERNAL_DATA_PLAN.md).
--
-- Cricsheet's `info.event` carries four things: `name`, `match_number`, `stage` and
-- `group`. The importer stored the first two and discarded the last two, so the database
-- could say *which competition* a match belonged to but never *which part of it* -- and
-- "which part of it" is precisely what a stakes derivation needs. Reconstructing a stage
-- from the event name instead does not work: the 1,102 distinct event names in the archive
-- name tournaments ("ICC Men's T20 World Cup Qualifier" is a qualifying *competition*,
-- every match in it included), not the round a match was played in.
--
-- `stage` is the round: 'Final', 'Semi Final', 'Qualifier 1', 'Eliminator',
-- '3rd Place Play-Off', 'Group Stage', 'Super Sixes', and about fifty more spellings.
-- It is present on 1,584 of 22,818 archived matches, and where a league has playoffs it is
-- present on exactly those playoffs (IPL: 74 of 1,243, the 1,169 league matches carrying a
-- match_number instead) -- which is why it is worth storing even at 7 % overall coverage.
--
-- `group` is the pool a group-stage match belonged to: 'A', 'B', 'South', '1', 'Elite
-- Group C'. Cricsheet writes it as a string in most files and as a bare number in others,
-- so the column is text and the importer normalises; 6,537 matches carry one.
--
-- Both are nullable and stay null where the archive says nothing. Nothing is inferred here:
-- the derivation that turns these into stage labels and a dead-rubber flag lives in the
-- rating pass (ml/xi/stakes.py), reads both sources through the same code, and leaves a
-- match it cannot label unlabelled.

BEGIN;

ALTER TABLE public.match ADD COLUMN IF NOT EXISTS event_stage character varying(64);
ALTER TABLE public.match ADD COLUMN IF NOT EXISTS event_group character varying(64);

COMMENT ON COLUMN public.match.event_stage IS
    'Cricsheet info.event.stage verbatim: the round of the competition, null where the archive names none';
COMMENT ON COLUMN public.match.event_group IS
    'Cricsheet info.event.group verbatim (numbers rendered as text): the pool of a group-stage match';

COMMIT;
