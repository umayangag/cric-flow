import { describe, it, expect } from 'vitest';
import { foldWindow, formatShare, formatStat, statMean, statSpread } from './evaluationReport';

describe('evaluationReport', () => {
  describe('statMean', () => {
    it('unwraps both shapes the report uses', () => {
      expect(statMean(0.75)).toBe(0.75);
      expect(statMean({ mean: 0.747, sd: 0.012, n_folds: 7 })).toBe(0.747);
    });

    it('treats a missing or non-finite number as absent', () => {
      expect(statMean(null)).toBeNull();
      expect(statMean(undefined)).toBeNull();
      expect(statMean(Number.NaN)).toBeNull();
      expect(statMean({ mean: Number.NaN, sd: 0, n_folds: 1 })).toBeNull();
    });
  });

  describe('statSpread', () => {
    it('is absent for a single fold, because there is no spread to report', () => {
      expect(statSpread(0.75)).toBeNull();
      expect(statSpread({ mean: 0.75, sd: 0.02, n_folds: 7 })).toBe(0.02);
    });
  });

  describe('formatStat', () => {
    it('shows the spread when the harness summarised over folds', () => {
      expect(formatStat({ mean: 0.747, sd: 0.012, n_folds: 7 })).toBe('0.747 ± 0.012');
    });

    it('shows the bare point for a single fold', () => {
      expect(formatStat(0.751)).toBe('0.751');
    });

    it('shows an em dash rather than a zero for a number the fold could not produce', () => {
      expect(formatStat(null)).toBe('—');
    });

    it('honours the requested precision', () => {
      expect(formatStat(90.62, 1)).toBe('90.6');
    });
  });

  describe('formatShare', () => {
    it('renders a share as a percentage', () => {
      expect(formatShare(0.786)).toBe('78.6%');
      expect(formatShare({ mean: 0.004, sd: 0, n_folds: 7 })).toBe('0.4%');
      expect(formatShare(undefined)).toBe('—');
    });
  });

  describe('foldWindow', () => {
    it('labels an open-ended window as running to today', () => {
      expect(foldWindow('2025-09-01', '9999-12-31')).toBe('2025-09-01 → today');
      expect(foldWindow('2025-09-01', '2262-04-11')).toBe('2025-09-01 → today');
    });

    it('shows a closed window as it is', () => {
      expect(foldWindow('2024-01-01', '2024-04-01')).toBe('2024-01-01 → 2024-04-01');
    });
  });
});
