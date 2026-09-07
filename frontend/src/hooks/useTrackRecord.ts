import { useMemo } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { EvaluationFormatReport, EvaluationReport, TrackRecord } from '../types';
import { statMean } from '../utils/evaluationReport';

/**
 * The harness's figures for one format, read off L4's report for the track record to show
 * beside its own (P2-4): a reader sees the record against the number the model was
 * accepted on. The locked window where it was scored, otherwise the walk-forward fold
 * means -- and the row says which. Both denominators of B-12's split are carried: the
 * pooled coverage and, where some fold shipped without a shared factor, the coverage over
 * the folds that had one.
 */
export type HarnessFigures = {
  source: 'locked window' | 'walk-forward fold means';
  n: number | null;
  displayBrier: number | null;
  baseRateBrier: number | null;
  firstInningsCoverage: number | null;
  chaseCoverage: number | null;
  /** Over the folds with a shared factor only; null where every fold had one. */
  firstInningsCoverageWithFactor: number | null;
  chaseCoverageWithFactor: number | null;
  foldsWithoutFactor: number;
};

export function harnessFiguresFor(report: EvaluationFormatReport): HarnessFigures {
  const locked = report.locked;
  const lockedScored = !locked.skipped_reason && (locked.n_eval ?? 0) > 0;
  const folds = report.walk_forward.summary;
  const simulation = lockedScored ? locked.simulation : folds.simulation;
  const split = folds.simulation?.shared_factor_folds;
  const withFactor =
    !lockedScored && split && split.without_shared_factor > 0
      ? split.totals_with_shared_factor
      : null;
  return {
    source: lockedScored ? 'locked window' : 'walk-forward fold means',
    n: lockedScored ? locked.n_eval : statMean(simulation?.n_matches),
    displayBrier: lockedScored
      ? statMean(locked.display_brier_mean)
      : statMean(simulation?.win?.brier?.display),
    baseRateBrier: lockedScored
      ? statMean(locked.base_rate_brier)
      : statMean(folds.base_rate_brier),
    firstInningsCoverage: statMean(simulation?.totals?.first_innings?.coverage_80),
    chaseCoverage: statMean(simulation?.totals?.chase?.coverage_80),
    firstInningsCoverageWithFactor: statMean(withFactor?.first_innings?.coverage_80),
    chaseCoverageWithFactor: statMean(withFactor?.chase?.coverage_80),
    foldsWithoutFactor: split?.without_shared_factor ?? 0,
  };
}

/**
 * The track record and, beside it, the harness's report. The two are fetched separately
 * and fail separately: a missing evaluation report leaves the record whole, with the
 * harness columns empty and the reason shown, because the record is a read of the store
 * and owes nothing to `make evaluate` having been run.
 */
export function useTrackRecord() {
  const record = useAsync(api.trackRecord, {
    runOnMount: [],
    errorMessage: 'Could not load the track record',
  });
  const harness = useAsync(api.evaluationReport, {
    runOnMount: [],
    errorMessage: 'No evaluation report to compare against',
  });
  const harnessReport = harness.data as EvaluationReport | null;
  const harnessByFormat = useMemo(() => {
    const out: Record<string, HarnessFigures> = {};
    for (const [format, report] of Object.entries(harnessReport?.formats ?? {})) {
      out[format] = harnessFiguresFor(report);
    }
    return out;
  }, [harnessReport]);

  return {
    record: record.data as TrackRecord | null,
    loading: record.loading,
    error: record.error,
    reload: record.run,
    harnessByFormat,
    harnessGeneratedAt: harnessReport?.generated_at ?? null,
    harnessError: harness.error,
  };
}
