import { describe, it, expect } from 'vitest';
import {
  asRunMetadata,
  compareMetrics,
  findPreviousRun,
  flattenMetrics,
  metricDirection,
  parseFailure,
} from './runMetadata';
import type { Migration } from '../types';

function run(overrides: Partial<Migration> & { id: number }): Migration {
  return {
    command: 'train-batting',
    args: {},
    started_at: '2026-08-26T12:00:00Z',
    status: 'COMPLETED',
    ...overrides,
  } as Migration;
}

function withMetrics(id: number, started: string, metrics: Record<string, number>): Migration {
  return run({
    id,
    started_at: started,
    metadata: { summary: { formats: [{ format: 'T20I', metrics }] } },
  });
}

describe('metricDirection', () => {
  // Getting this backwards would be worse than showing no verdict at all: it would
  // report a regression as an improvement.
  it('knows which way is good for the metrics the trainers emit', () => {
    for (const name of [
      'rmse',
      'player_runs_mae',
      'cv_brier_mean',
      'log_loss',
      'cv_accuracy_std',
    ]) {
      expect(metricDirection(name), name).toBe('lower-is-better');
    }
    for (const name of ['accuracy', 'cv_accuracy_mean', 'r2', 'best_score', 'auc']) {
      expect(metricDirection(name), name).toBe('higher-is-better');
    }
  });

  // A spread name contains the quantity it measures ("accuracy", "r2"), and read as
  // higher-is-better it would report a model that got *less* consistent as an
  // improvement. Spread beats the quantity it measures.
  it('treats a spread measure as lower-is-better even when it names a good metric', () => {
    expect(metricDirection('cv_accuracy_std')).toBe('lower-is-better');
    expect(metricDirection('score_stddev')).toBe('lower-is-better');
    expect(metricDirection('r2_variance')).toBe('lower-is-better');
  });

  // L-1: the service that computes a metric says which way is progress. The substring
  // rule stays only for the keys the glossary does not carry.
  it('takes the direction from the glossary where it carries the key', () => {
    const lookup = (key: string | undefined) =>
      key === 'objective_auc'
        ? ({ direction: 'higher' } as never)
        : key === 'coverage_80'
          ? ({ direction: 'nominal' } as never)
          : undefined;

    expect(metricDirection('T20.objective_auc', lookup)).toBe('higher-is-better');
    // Nominal: neither up nor down is an improvement, so no verdict is claimed.
    expect(metricDirection('T20.coverage_80', lookup)).toBe('unknown');
    // A key the glossary does not carry still falls back to the substring rule.
    expect(metricDirection('T20I.rmse', lookup)).toBe('lower-is-better');
  });

  it('declines to guess for a metric it does not recognise', () => {
    expect(metricDirection('rows')).toBe('unknown');
    expect(metricDirection('duration_seconds')).toBe('unknown');
  });
});

describe('compareMetrics', () => {
  it('calls a lower rmse better and a higher one worse', () => {
    const [improved] = compareMetrics({ rmse: 22.0 }, { rmse: 24.0 });
    expect(improved.verdict).toBe('better');
    expect(improved.delta).toBe(-2);
    expect(improved.deltaPct).toBeCloseTo(-2 / 24);

    const [regressed] = compareMetrics({ rmse: 26.0 }, { rmse: 24.0 });
    expect(regressed.verdict).toBe('worse');
  });

  it('calls a higher accuracy better', () => {
    const [c] = compareMetrics({ accuracy: 0.75 }, { accuracy: 0.7 });
    expect(c.verdict).toBe('better');
    expect(c.delta).toBeCloseTo(0.05);
  });

  it('shows the change without a verdict when the direction is unknown', () => {
    const [c] = compareMetrics({ rows: 1500 }, { rows: 1200 });
    expect(c.delta).toBe(300);
    expect(c.verdict).toBe('unknown');
  });

  it('reports an unchanged metric as the same, not as an improvement', () => {
    const [c] = compareMetrics({ rmse: 24.0 }, { rmse: 24.0 });
    expect(c.verdict).toBe('same');
    expect(c.delta).toBe(0);
  });

  it('has no delta when there is nothing to compare against', () => {
    const [c] = compareMetrics({ rmse: 24.0 }, undefined);
    expect(c.delta).toBeUndefined();
    expect(c.verdict).toBe('unknown');

    const [missing] = compareMetrics({ rmse: 24.0 }, { accuracy: 0.7 });
    expect(missing.delta).toBeUndefined();
  });

  // A percentage against zero is a division by zero, and Infinity rendered as a
  // change is worse than no percentage.
  it('omits the percentage when the previous value was zero', () => {
    const [c] = compareMetrics({ rmse: 5 }, { rmse: 0 });
    expect(c.delta).toBe(5);
    expect(c.deltaPct).toBeUndefined();
  });

  it('sorts by name so the table does not reshuffle between runs', () => {
    const names = compareMetrics({ zeta: 1, alpha: 2, mid: 3 }).map((c) => c.name);
    expect(names).toEqual(['alpha', 'mid', 'zeta']);
  });
});

describe('flattenMetrics', () => {
  // T20I.rmse and ODI.rmse are different numbers; merging would silently keep
  // whichever came last.
  it('prefixes by format rather than merging', () => {
    const flat = flattenMetrics({
      summary: {
        formats: [
          { format: 'T20I', metrics: { rmse: 24.1 } },
          { format: 'ODI', metrics: { rmse: 31.7 } },
        ],
      },
    });
    expect(flat).toEqual({ 'T20I.rmse': 24.1, 'ODI.rmse': 31.7 });
  });

  it('drops values that are not finite numbers', () => {
    const flat = flattenMetrics({
      summary: {
        formats: [{ format: 'T20I', metrics: { good: 1, bad: NaN, worse: Infinity } as never }],
      },
    });
    expect(flat).toEqual({ 'T20I.good': 1 });
  });

  it('is empty for a run with no summary', () => {
    expect(flattenMetrics(null)).toEqual({});
    expect(flattenMetrics({})).toEqual({});
  });
});

describe('findPreviousRun', () => {
  const current = withMetrics(10, '2026-08-26T12:00:00Z', { rmse: 24 });

  it('picks the most recent earlier completed run of the same command', () => {
    const found = findPreviousRun(current, [
      current,
      withMetrics(8, '2026-08-25T12:00:00Z', { rmse: 25 }),
      withMetrics(9, '2026-08-26T09:00:00Z', { rmse: 26 }),
    ]);
    expect(found?.id).toBe(9);
  });

  it('never compares against a different step', () => {
    const found = findPreviousRun(current, [
      { ...withMetrics(9, '2026-08-26T09:00:00Z', { rmse: 26 }), command: 'train-bowling' },
    ]);
    expect(found).toBeUndefined();
  });

  it('never compares against a later run', () => {
    const found = findPreviousRun(current, [withMetrics(11, '2026-08-27T09:00:00Z', { rmse: 1 })]);
    expect(found).toBeUndefined();
  });

  // A failed run's numbers are not a baseline.
  it('skips runs that did not complete', () => {
    const failed = withMetrics(9, '2026-08-26T09:00:00Z', { rmse: 26 });
    failed.status = 'FAILED';
    expect(findPreviousRun(current, [failed])).toBeUndefined();
  });

  it('skips earlier runs that recorded no metrics', () => {
    const noMetrics = run({ id: 9, started_at: '2026-08-26T09:00:00Z', metadata: {} });
    expect(findPreviousRun(current, [noMetrics])).toBeUndefined();
  });

  it('returns nothing rather than comparing against itself', () => {
    expect(findPreviousRun(current, [current])).toBeUndefined();
  });
});

describe('parseFailure', () => {
  // go-app formats an ml-service precondition as `CODE: message — hint` precisely so
  // the operator is told what to do next.
  it('splits a coded failure into code, message and hint', () => {
    const parsed = parseFailure(
      'CONTRIBUTIONS_CSV_MISSING: backtest_contributions.csv not found — Run POST /api/backtest/export-contributions first',
    );
    expect(parsed).toEqual({
      code: 'CONTRIBUTIONS_CSV_MISSING',
      message: 'backtest_contributions.csv not found',
      hint: 'Run POST /api/backtest/export-contributions first',
    });
  });

  it('handles a coded failure with no hint', () => {
    const parsed = parseFailure('BAD_INPUT: cutoff is required');
    expect(parsed).toEqual({ code: 'BAD_INPUT', message: 'cutoff is required' });
  });

  it('passes an unstructured message through unchanged', () => {
    const raw = 'dial tcp 127.0.0.1:8000: connect: connection refused';
    expect(parseFailure(raw)).toEqual({ message: raw });
  });

  // An en dash or hyphen inside the message must not be mistaken for the em-dash
  // separator Error() writes.
  it('does not split on a hyphen inside the message', () => {
    const parsed = parseFailure('SOME_CODE: value must be non-negative');
    expect(parsed?.message).toBe('value must be non-negative');
    expect(parsed?.hint).toBeUndefined();
  });

  it('is null for an absent or blank message', () => {
    expect(parseFailure(undefined)).toBeNull();
    expect(parseFailure('   ')).toBeNull();
  });
});

describe('asRunMetadata', () => {
  it('accepts an object and rejects everything else', () => {
    expect(asRunMetadata({ step: 'train_batting' })).toEqual({ step: 'train_batting' });
    expect(asRunMetadata(null)).toBeNull();
    expect(asRunMetadata(undefined)).toBeNull();
    expect(asRunMetadata('a string')).toBeNull();
    expect(asRunMetadata([1, 2])).toBeNull();
  });
});
