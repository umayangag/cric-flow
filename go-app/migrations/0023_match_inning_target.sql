-- 0023_match_inning_target.sql
--
-- Hold the over limit a chase was given beside the runs it needed (IMPORT-11 in
-- docs/AUDIT_FINDINGS.md).
--
-- Cricsheet writes `innings[].target` as {runs, overs} on the innings being chased: `runs`
-- is the score that wins, `overs` is the limit that chase was given. 18,264 of the 22,905
-- files in the archive carry one. The importer read neither. It wrote the first innings'
-- total into target_runs -- one short of the score that wins, on every one of the 19,432
-- limited-overs second innings in the table -- and it wrote that number onto the 3,102
-- second innings of Tests and first-class matches too, where there is no target at all and
-- the first innings' total is not even the lead.
--
-- The loss that matters is the revised target. On 983 files `target.runs` is not
-- first + 1 and on 1,541 `target.overs` is not the scheduled allotment, because rain cut
-- the chase; a chase of 235 in 42 overs was stored as 270 in nothing. `match.result_method`
-- cannot recover them: it is `info.outcome.method`, which says how the *result* was
-- reached, and 577 of the 1,550 revised-target matches name no method there at all because
-- the revised chase was completed normally. So the over limit needs its own column; the
-- method does not, and is not duplicated here.
--
-- real, matching overs_bowled: the archive's over figures are the scorer's O.B notation
-- (12.4 is twelve overs and four balls), and 158 of these targets are fractional.
-- Nullable, because most innings are not a chase and because a chase whose file names no
-- target keeps its allotment in match.scheduled_overs_per_innings.
--
-- Nothing back-fills either column. The rows already in the table keep the wrong
-- target_runs and a null target_overs until the directory is re-imported, which is what
-- writes them -- and what clears the 3,102 multi-day rows to null, since the upsert
-- assigns target_runs = EXCLUDED.target_runs on conflict.

BEGIN;

ALTER TABLE public.match_inning ADD COLUMN IF NOT EXISTS target_overs real;

COMMENT ON COLUMN public.match_inning.target_runs IS
    'Cricsheet innings[].target.runs: the score that wins this innings, revised figure included; first innings total + 1 where the file names no target and the innings is a limited-overs chase; null for an innings that is not a chase. Wrong by one, and set on multi-day innings, until the match is re-imported (IMPORT-11)';
COMMENT ON COLUMN public.match_inning.target_overs IS
    'Cricsheet innings[].target.overs: the over limit this chase was given, in O.B notation, cut where rain revised it; null where the file names none (the allotment is then match.scheduled_overs_per_innings) and until the match is re-imported (IMPORT-11)';

COMMIT;
