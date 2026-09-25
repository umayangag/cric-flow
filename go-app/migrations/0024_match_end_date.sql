-- 0024_match_end_date.sql
--
-- Record the day a match ended beside the day it started (FEAT-09 in
-- docs/AUDIT_FINDINGS.md).
--
-- `match.match_date` is Cricsheet's `dates[0]`, and it is the only date the row holds.
-- 3,171 of the 22,905 files in the archive list more than one day -- 918 Tests, 2,207
-- first-class rounds, 46 one-day or twenty-over matches carried over -- and the rating
-- pass, which folds a match into its state at the close of its date, folded every one of
-- them at the close of its first day. A fixture played during a Test therefore read a
-- state holding that Test's later days: 10,656 matches start inside another match's span,
-- 1,386 of them in the same format.
--
-- `match_end_date` is `dates[-1]` verbatim -- the archive lists the days in order in every
-- file -- and the pass now folds a match at the close of that day. It is nullable because
-- nothing back-fills it: the rating pass reads COALESCE(match_end_date, match_date), so a
-- database migrated but not yet re-imported behaves exactly as before, and `make xi-parity`
-- reports it against the archive as `multi_day_matches` differing. The re-import of the
-- whole directory is what writes it.

BEGIN;

ALTER TABLE public.match ADD COLUMN IF NOT EXISTS match_end_date date;

COMMENT ON COLUMN public.match.match_end_date IS
    'The last day the match was played on (Cricsheet dates[-1]); match_date is the first. NULL until the file is re-imported';

COMMIT;
