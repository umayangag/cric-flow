import type { FoldStat, HoldoutRecord } from '../types';

/**
 * The report's numeric leaves come in two shapes: a bare number on a single fold (the
 * locked window), and `{mean, sd, n_folds}` where the harness summarised over folds. Every
 * reader needs both, so the unwrapping lives here rather than at each call site.
 */
export type ReportNumber = number | FoldStat | null | undefined;

export function statMean(value: ReportNumber): number | null {
  if (value == null) return null;
  if (typeof value === 'number') return Number.isFinite(value) ? value : null;
  return Number.isFinite(value.mean) ? value.mean : null;
}

export function statSpread(value: ReportNumber): number | null {
  if (value == null || typeof value === 'number') return null;
  return Number.isFinite(value.sd) ? value.sd : null;
}

/** "0.747 ± 0.012", or "0.747" where there is one fold and so no spread. */
export function formatStat(value: ReportNumber, digits = 3): string {
  const mean = statMean(value);
  if (mean == null) return '—';
  const spread = statSpread(value);
  const point = mean.toFixed(digits);
  return spread == null ? point : `${point} ± ${spread.toFixed(digits)}`;
}

/**
 * EVAL-11: where a fold summary came from — "over 11 folds, read by 29 gates" — rendered
 * beside the number so a development figure is never mistaken for a holdout one. Absent
 * for a bare number, which is a single window's figure.
 */
export function foldProvenance(value: ReportNumber): string | null {
  if (value == null || typeof value === 'number') return null;
  const folds = `over ${value.n_folds} fold${value.n_folds === 1 ? '' : 's'}`;
  return value.gates_consulted == null ? folds : `${folds}, read by ${value.gates_consulted} gates`;
}

/** EVAL-11: the holdout's own label — "19 of 365 season days, incomplete, read by no gate". */
export function holdoutSeason(record: HoldoutRecord): string {
  const accrued = `${record.days_covered} of ${record.season_days} season days`;
  const state = record.season_complete ? 'complete' : 'incomplete';
  return `${accrued}, ${state}, read by no gate`;
}

/** A share as a percentage: "0.4%". */
export function formatShare(value: ReportNumber, digits = 1): string {
  const mean = statMean(value);
  return mean == null ? '—' : `${(mean * 100).toFixed(digits)}%`;
}

/** A fold's window, as the report labels it: "2024-01-01 → 2024-04-01". */
export function foldWindow(cutoff: string, end: string): string {
  const to = end.startsWith('9999') || end.startsWith('2262') ? 'today' : end;
  return `${cutoff} → ${to}`;
}
