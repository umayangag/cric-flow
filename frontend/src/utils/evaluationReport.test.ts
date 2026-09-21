import { describe, it, expect } from 'vitest';
import {
  foldProvenance,
  foldWindow,
  formatShare,
  formatStat,
  holdoutSeason,
  statMean,
  statSpread,
} from './evaluationReport';

describe('evaluationReport', () => {
  describe('foldProvenance', () => {
    it('names the folds and how many gates read them, beside a development number', () => {
      expect(foldProvenance({ mean: 0.697, sd: 0.038, n_folds: 11, gates_consulted: 29 })).toBe(
        'over 11 folds, read by 29 gates',
      );
    });

    it('names the folds alone when the report predates the gate count', () => {
      expect(foldProvenance({ mean: 0.7, sd: 0.01, n_folds: 1 })).toBe('over 1 fold');
    });

    it('is absent for a bare number, which is one window and not a summary', () => {
      expect(foldProvenance(0.788)).toBeNull();
      expect(foldProvenance(null)).toBeNull();
    });
  });

  describe('holdoutSeason', () => {
    const record = {
      n_matches: 52,
      first_match: '2026-09-02',
      last_match: '2026-09-20',
      season_start: '2026-09-02',
      season_end: '2027-09-02',
      season_days: 365,
      days_covered: 19,
      season_complete: false,
      gates_consulted: 0,
    };

    it('says how much of the season has accrued and that no gate read it', () => {
      expect(holdoutSeason(record)).toBe('19 of 365 season days, incomplete, read by no gate');
    });

    it('says when the season is complete', () => {
      expect(holdoutSeason({ ...record, days_covered: 365, season_complete: true })).toBe(
        '365 of 365 season days, complete, read by no gate',
      );
    });
  });

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
