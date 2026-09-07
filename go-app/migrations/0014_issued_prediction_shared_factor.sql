-- 0014_issued_prediction_shared_factor.sql
--
-- The track record (P2-4) keeps two simulator populations apart.
--
-- A format whose calibration fold is too thin to fit the shared match factor ships the
-- un-widened simulator, and its 10-90 intervals belong to a different model from the
-- factored one's (B-12: 0.756 pooled against 0.774 over the calibrated ODI folds). The
-- harness now reports the two separately, and a record scored from served answers has to
-- do the same -- which means every stored answer has to say which simulator served it.
-- ml-service puts it on `/simulate`, go-app carries it onto the scorecard, and this column
-- keeps it beside the other grouping columns so the record can split without parsing a
-- payload. It is also inside `payload` (`scorecard.shared_factor`), as every column here is.
--
-- NULL means one of two things the payload tells apart: the answer was stored before this
-- column existed (a third population, reported as "unknown"), or no simulator ran because
-- the format has no innings length (no scorecard, no ranges, nothing to cover). Rows from
-- before this migration are left NULL on purpose -- the record does not know what served
-- them, and a backfilled guess would be exactly the pooling the column exists to prevent.

BEGIN;

ALTER TABLE public.issued_prediction
    ADD COLUMN IF NOT EXISTS simulator_shared_factor boolean;

COMMENT ON COLUMN public.issued_prediction.simulator_shared_factor IS
    'Whether the simulator that served this answer carried a shared match factor (B-12); NULL before this column existed, or where no simulator ran';

COMMIT;
