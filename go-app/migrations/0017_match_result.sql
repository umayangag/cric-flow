-- 0017_match_result.sql
--
-- How a match was decided (IMPORT-02 in docs/AUDIT_FINDINGS.md).
--
-- Cricsheet's `info.outcome` writes a match that was won outright as `winner` (plus the
-- margin), and a match that was not as `result` -- 'draw', 'no result' or 'tie' -- with no
-- winner. A tie that a tie-breaker then settled keeps `result: tie` and names the side that
-- won the tie-breaker under `eliminator` (a super over) or `bowl_out`. The importer read
-- only `winner`, so a super-over win was stored with `outcome_winner_opposition_id` NULL:
-- the same row as an abandoned match, a drawn Test or a tie nobody broke, and the rating
-- pass -- which reads a NULL winner as "no result" -- left every one of them out of the
-- frame and out of both sides' Elo.
--
-- Two columns, both nullable and both Cricsheet's own words:
--
-- `result` is `outcome.result` verbatim. It is null for a match won outright, because
-- the archive writes none there, and it is *kept* beside a winner for a tie-breaker win:
-- `outcome_winner_opposition_id` now comes from `winner`, else `eliminator`, else
-- `bowl_out`, so a winner with `result = 'tie'` is a match that was tied and then
-- decided, distinguishable from an outright win and from a tie that was left as one.
-- Nothing back-fills it: a re-import of the whole directory is what writes it.
--
-- `result_method` is `outcome.method`: the rule that adjusted or awarded the result. In
-- the current archive it is 'D/L' (1,018 files), 'VJD' (5), 'Awarded' (5) and, once,
-- 'Lost fewer wickets' -- eighteen characters, which is why the column is wider than the
-- sixteen the finding proposed: varchar(16) would have refused that file outright.

BEGIN;

ALTER TABLE public.match ADD COLUMN IF NOT EXISTS result character varying(16);
ALTER TABLE public.match ADD COLUMN IF NOT EXISTS result_method character varying(32);

COMMENT ON COLUMN public.match.result IS
    'Cricsheet info.outcome.result verbatim (draw | no result | tie); null for a match won outright. A winner beside ''tie'' is a tie-breaker win';
COMMENT ON COLUMN public.match.result_method IS
    'Cricsheet info.outcome.method verbatim (D/L, VJD, Awarded, ...): the rule that adjusted or awarded the result; null where the archive names none';

COMMIT;
