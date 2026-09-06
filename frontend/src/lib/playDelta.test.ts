import { describe, it, expect } from 'vitest';
import {
  changeDirection,
  formatChange,
  formatProbabilityChange,
  isSameFixture,
  playDelta,
} from './playDelta';
import type { PredictTeamSelectionResponse } from '../types';

/**
 * The only arithmetic the Lab does on a served number (P1-2): one answer's value minus
 * another's, between two answers to the same question.
 */

const pool = {
  source: 'recency_window' as const,
  window_months: 9,
  since: '2025-12-10',
  size: 24,
  retired_excluded: 0,
};

function answer(
  overrides: Partial<PredictTeamSelectionResponse> = {},
): PredictTeamSelectionResponse {
  return {
    ratings_through: '2026-09-02',
    run_id: '20260906T083819Z-36689f80',
    team1_side: { club_id: 43, name: 'India', gender: 'male', display_name: 'India (men)' },
    team2_side: { club_id: 7, name: 'Australia', gender: 'male', display_name: 'Australia (men)' },
    team1: [],
    team2: [],
    selection: { objective: 'fixed', optimised: false },
    forecast: { source: 'simulator' },
    win_probability: { team1: 0.6, source: 'display', predicted_winner: 'India (men)' },
    toss: { team1_bats_first: null, honoured: true },
    scorecard: {
      samples: 2000,
      toss_marginalised: true,
      team1_innings: { total: 176, extras: 8, p10: 140, median: 175, p90: 212 },
      team2_innings: { total: 175, extras: 9, p10: 139, median: 174, p90: 210 },
    },
    team1_pool: pool,
    team2_pool: pool,
    ...overrides,
  };
}

describe('playDelta', () => {
  it('subtracts the previous answer from the current one', () => {
    const previous = answer();
    const current = answer({
      win_probability: { team1: 0.634, source: 'display', predicted_winner: 'India (men)' },
      scorecard: {
        samples: 2000,
        toss_marginalised: true,
        team1_innings: { total: 182, extras: 8, p10: 145, median: 181, p90: 220 },
        team2_innings: { total: 175, extras: 9, p10: 139, median: 174, p90: 210 },
      },
    });

    const delta = playDelta(previous, current);

    expect(delta?.winProbability).toEqual({ previous: 0.6, current: 0.634, change: 0.634 - 0.6 });
    expect(delta?.team1Innings?.total.change).toBeCloseTo(6, 10);
    expect(delta?.team1Innings?.p10.change).toBeCloseTo(5, 10);
    expect(delta?.team1Innings?.p90.change).toBeCloseTo(8, 10);
    expect(delta?.team2Innings?.total.change).toBeCloseTo(0, 10);
  });

  it('has no innings to compare where the format has no simulated match', () => {
    const previous = answer({ scorecard: undefined });
    const current = answer({ scorecard: undefined });

    const delta = playDelta(previous, current);

    expect(delta?.team1Innings).toBeNull();
    expect(delta?.team2Innings).toBeNull();
    expect(delta?.winProbability.change).toBe(0);
  });

  it('is nothing at all until there are two answers to compare', () => {
    expect(playDelta(null, answer())).toBeNull();
    expect(playDelta(answer(), null)).toBeNull();
  });

  // A difference across two different questions is not the change the user made.
  it('refuses to compare two answers to different questions', () => {
    const previous = answer();

    expect(
      playDelta(
        previous,
        answer({
          team2_side: {
            club_id: 9,
            name: 'England',
            gender: 'male',
            display_name: 'England (men)',
          },
        }),
      ),
    ).toBeNull();
    expect(
      playDelta(previous, answer({ toss: { team1_bats_first: true, honoured: true } })),
    ).toBeNull();
    expect(playDelta(previous, answer({ run_id: 'a-later-run' }))).toBeNull();
    expect(playDelta(previous, answer({ ratings_through: '2026-09-09' }))).toBeNull();
  });
});

describe('isSameFixture', () => {
  it('is true for two answers to the same fixture from the same run', () => {
    expect(isSameFixture(answer(), answer())).toBe(true);
  });
});

describe('formatChange', () => {
  it('signs a change and leaves an unchanged number unsigned', () => {
    expect(formatChange(6.04, 1)).toBe('+6.0');
    expect(formatChange(-6.04, 1)).toBe('−6.0');
    expect(formatChange(0, 1)).toBe('0.0');
  });

  it('does not sign a change that rounds away', () => {
    expect(formatChange(0.004, 1)).toBe('0.0');
    expect(formatChange(-0.004, 1)).toBe('0.0');
  });
});

describe('formatProbabilityChange', () => {
  it('shows a probability change in percentage points', () => {
    expect(formatProbabilityChange(0.034)).toBe('+3.4 pp');
    expect(formatProbabilityChange(-0.012)).toBe('−1.2 pp');
    expect(formatProbabilityChange(0)).toBe('0.0 pp');
  });
});

describe('changeDirection', () => {
  it('names the direction a change points', () => {
    expect(changeDirection(0.1)).toBe('up');
    expect(changeDirection(-0.1)).toBe('down');
    expect(changeDirection(0)).toBe('none');
  });
});
