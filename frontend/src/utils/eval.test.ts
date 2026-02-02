import { describe, it, expect } from 'vitest';
import { computeMetrics, determineImmediateNextSeason, inferSeasonFromDate } from './eval';

describe('inferSeasonFromDate', () => {
  it('extracts year from ISO date', () => {
    expect(inferSeasonFromDate('2023-05-01')).toBe(2023);
    expect(inferSeasonFromDate('2019-12-31')).toBe(2019);
  });
});

describe('determineImmediateNextSeason', () => {
  it('returns the smallest season strictly greater than cutoff year', () => {
    const seasons = [2019, 2020, 2021, 2023];
    expect(determineImmediateNextSeason('2020-06-01', seasons)).toBe(2021);
    expect(determineImmediateNextSeason('2022-01-01', seasons)).toBe(2023);
  });

  it('returns null if none greater', () => {
    expect(determineImmediateNextSeason('2025-01-01', [2022, 2023, 2024])).toBeNull();
  });
});

describe('computeMetrics', () => {
  it('computes accuracy and confusion matrix', () => {
    const preds = [
      { prob: 0.9, actual: 1 },
      { prob: 0.7, actual: 1 },
      { prob: 0.2, actual: 0 },
      { prob: 0.4, actual: 1 },
    ];
    const m = computeMetrics(preds, 0.5);
    expect(m.total).toBe(4);
    expect(m.confusion.tp).toBe(2); // 0.9, 0.7 vs actual 1
    expect(m.confusion.tn).toBe(1); // 0.2 vs actual 0
    expect(m.confusion.fp).toBe(0); // none predicted 1 when actual 0
    expect(m.confusion.fn).toBe(1); // 0.4 predicted 0 but actual 1
    expect(m.correct).toBe(3);
    expect(m.accuracy).toBeCloseTo(0.75, 6);
  });
});
