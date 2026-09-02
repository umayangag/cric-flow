import type { MetricGlossaryEntry, Migration, RunMetadata } from '../types';

/**
 * Reading a finished run's recorded metadata (ops plan O-4/O-5).
 *
 * The comparison against a previous run is the part with judgement in it: whether a
 * number moving up is good news depends entirely on which number it is, and getting
 * that backwards would be worse than showing no comparison at all.
 */

/** Metadata shaped by O-4, or null for a row that carries something else. */
export function asRunMetadata(metadata: unknown): RunMetadata | null {
  if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) return null;
  return metadata as RunMetadata;
}

/**
 * Spread measures, where less is better whatever the underlying quantity is.
 *
 * Checked *first*, and that ordering is load-bearing: a name like `display_auc_sd`
 * contains "auc" and would otherwise be read as higher-is-better — reporting a model
 * that got less consistent across seeds as an improvement.
 */
const SPREAD = ['_std', 'stddev', 'variance', 'spread'];

/**
 * Metrics where a *lower* value is the better one.
 *
 * Matched by substring on the metric name, because the trainers name them
 * per-target (`runs_pinball`, `display_auc_mean`) rather than from a fixed list.
 * A metric that matches nothing here is shown with its delta and no verdict —
 * "it changed by this much" is true regardless, while "this is an improvement"
 * would be a guess.
 *
 * This is the *fallback*. Where the glossary carries the key it decides (L-1): the
 * service that computes a metric knows which way is progress, and a substring rule
 * guessing alongside it is a second answer waiting to disagree.
 */
const LOWER_IS_BETTER = ['rmse', 'mae', 'error', 'loss', 'brier', 'dropped'];

/** Metrics where a *higher* value is the better one. */
const HIGHER_IS_BETTER = ['accuracy', 'r2', 'score', 'auc', 'precision', 'recall', 'f1'];

export type MetricDirection = 'lower-is-better' | 'higher-is-better' | 'unknown';

/** Resolves a metric key against the served glossary; see `MetricGlossaryContext`. */
export type GlossaryLookup = (key: string | undefined) => MetricGlossaryEntry | undefined;

/**
 * Which way is good for this metric, or 'unknown' when we should not claim.
 *
 * `entry` is the glossary's word on it and wins where there is one. `nominal`, `exact`
 * and `none` yield no verdict on purpose: a coverage moving from 0.83 to 0.86 is not an
 * improvement, and colouring it green would say it was.
 */
export function metricDirection(name: string, lookup?: GlossaryLookup): MetricDirection {
  // Run metrics are prefixed by format (`T20.objective_auc`); the glossary knows the key.
  const entry = lookup?.(name.split('.').pop() ?? name);
  if (entry?.direction === 'higher') return 'higher-is-better';
  if (entry?.direction === 'lower') return 'lower-is-better';
  if (entry) return 'unknown';

  const key = name.toLowerCase();
  if (SPREAD.some((s) => key.includes(s))) return 'lower-is-better';
  if (HIGHER_IS_BETTER.some((h) => key.includes(h))) return 'higher-is-better';
  if (LOWER_IS_BETTER.some((l) => key.includes(l))) return 'lower-is-better';
  return 'unknown';
}

export type MetricComparison = {
  name: string;
  current: number;
  previous?: number;
  /** current - previous, absent when there is nothing to compare against. */
  delta?: number;
  /** Fractional change, absent when previous is 0 or missing. */
  deltaPct?: number;
  /** 'better' | 'worse' | 'same', or 'unknown' when the metric has no known direction. */
  verdict: 'better' | 'worse' | 'same' | 'unknown';
};

/** Compare one run's metrics against the previous run's, metric by metric. */
export function compareMetrics(
  current: Record<string, number>,
  previous?: Record<string, number>,
  lookup?: GlossaryLookup,
): MetricComparison[] {
  return Object.entries(current)
    .map(([name, value]) => {
      const before = previous?.[name];
      if (before === undefined || !Number.isFinite(before)) {
        return { name, current: value, verdict: 'unknown' as const };
      }
      const delta = value - before;
      const direction = metricDirection(name, lookup);
      let verdict: MetricComparison['verdict'] = 'unknown';
      if (delta === 0) {
        verdict = 'same';
      } else if (direction === 'lower-is-better') {
        verdict = delta < 0 ? 'better' : 'worse';
      } else if (direction === 'higher-is-better') {
        verdict = delta > 0 ? 'better' : 'worse';
      }
      return {
        name,
        current: value,
        previous: before,
        delta,
        deltaPct: before === 0 ? undefined : delta / Math.abs(before),
        verdict,
      };
    })
    .sort((a, b) => a.name.localeCompare(b.name));
}

/**
 * Flatten a run's per-format metrics into one map, prefixed by format.
 *
 * Prefixed rather than merged: `T20I.rmse` and `ODI.rmse` are different numbers, and
 * a merge would silently keep whichever came last.
 */
export function flattenMetrics(meta: RunMetadata | null): Record<string, number> {
  const out: Record<string, number> = {};
  for (const fmt of meta?.summary?.formats ?? []) {
    for (const [name, value] of Object.entries(fmt.metrics ?? {})) {
      if (typeof value === 'number' && Number.isFinite(value)) {
        out[`${fmt.format}.${name}`] = value;
      }
    }
  }
  return out;
}

/**
 * The most recent completed run of the same command, before this one.
 *
 * Returns undefined when the window holds no earlier run of that command — which is
 * reported as "no comparison available" rather than compared against an unrelated
 * run, or against one that failed and has no metrics worth the name.
 */
export function findPreviousRun(
  current: Migration,
  candidates: Migration[],
): Migration | undefined {
  const startedBefore = (m: Migration) =>
    new Date(m.started_at).getTime() < new Date(current.started_at).getTime();

  return candidates
    .filter(
      (m) =>
        m.id !== current.id &&
        m.command === current.command &&
        m.status === 'COMPLETED' &&
        startedBefore(m) &&
        Object.keys(flattenMetrics(asRunMetadata(m.metadata))).length > 0,
    )
    .sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())[0];
}

/**
 * A structured failure, when `error_message` carries one.
 *
 * go-app formats an ml-service precondition failure as `CODE: message — hint`
 * (`pipelinesvc.MLError.Error`), precisely so the operator is told what to do next.
 * Splitting it back out is presentation only: the string is already actionable, this
 * just stops the hint being buried in the middle of a red monospace block.
 */
export type ParsedFailure = { code?: string; message: string; hint?: string };

const CODED_FAILURE = /^([A-Z][A-Z0-9_]{2,}):\s*(.*)$/s;

export function parseFailure(errorMessage?: string): ParsedFailure | null {
  const raw = (errorMessage ?? '').trim();
  if (!raw) return null;

  const matched = CODED_FAILURE.exec(raw);
  if (!matched) return { message: raw };

  const [, code, rest] = matched;
  // The em dash is what Error() puts between message and hint. An en dash or hyphen
  // inside the message itself must not be mistaken for it.
  const separator = rest.indexOf(' — ');
  if (separator === -1) return { code, message: rest.trim() };
  return {
    code,
    message: rest.slice(0, separator).trim(),
    hint: rest.slice(separator + 3).trim(),
  };
}
