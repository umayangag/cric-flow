import type { PredictTeamSelectionResponse } from '../types';

/**
 * What one Play-mode change did to the numbers (P1-2).
 *
 * These are the only arithmetic the Lab does on a served number, and they are subtraction:
 * every value they subtract came from a response, and the difference is labelled as a
 * difference. Nothing here recomputes a probability, widens a range or averages two
 * answers — the two answers are two predictions, each true of the eleven that produced it.
 */

/** One number's move between two scored states. */
export type Delta = {
  previous: number;
  current: number;
  change: number;
};

function delta(previous: number, current: number): Delta {
  return { previous, current, change: current - previous };
}

/** An innings' move: the median-band total the scorecard sums to, and its 10-90 range. */
export type InningsDelta = {
  total: Delta;
  p10: Delta;
  p90: Delta;
};

/**
 * The whole re-score's move: the headline probability, and each side's innings where the
 * format has one to draw.
 *
 * `winProbability` is team 1's, as the response reports it. It is in probability units,
 * not percentage points — the surface formats it, and doing that twice is how a 1.4 %
 * change becomes 140 %.
 */
export type PlayDelta = {
  winProbability: Delta;
  team1Innings: InningsDelta | null;
  team2Innings: InningsDelta | null;
};

/**
 * The move from one scored state to the next, or null when there is nothing to compare.
 *
 * A comparison across two *different* fixtures is refused rather than shown: the previous
 * answer's teams, format or toss changed, so the difference between the two numbers is not
 * the change the user just made. It is also refused across a rating run, because a
 * probability that moved because the ratings were reloaded did not move because a player
 * was swapped (P1-5's stamp is what makes that checkable).
 */
export function playDelta(
  previous: PredictTeamSelectionResponse | null,
  current: PredictTeamSelectionResponse | null,
): PlayDelta | null {
  if (!previous || !current) return null;
  if (!isSameFixture(previous, current)) return null;
  return {
    winProbability: delta(previous.win_probability.team1, current.win_probability.team1),
    team1Innings: inningsDelta(previous, current, 'team1_innings'),
    team2Innings: inningsDelta(previous, current, 'team2_innings'),
  };
}

/** Whether two answers describe the same question, so their difference means something. */
export function isSameFixture(
  previous: PredictTeamSelectionResponse,
  current: PredictTeamSelectionResponse,
): boolean {
  return (
    previous.team1_side.club_id === current.team1_side.club_id &&
    previous.team2_side.club_id === current.team2_side.club_id &&
    previous.toss.team1_bats_first === current.toss.team1_bats_first &&
    previous.run_id === current.run_id &&
    previous.ratings_through === current.ratings_through
  );
}

function inningsDelta(
  previous: PredictTeamSelectionResponse,
  current: PredictTeamSelectionResponse,
  side: 'team1_innings' | 'team2_innings',
): InningsDelta | null {
  const before = previous.scorecard?.[side];
  const after = current.scorecard?.[side];
  if (!before || !after) return null;
  return {
    total: delta(before.total, after.total),
    p10: delta(before.p10, after.p10),
    p90: delta(before.p90, after.p90),
  };
}

/**
 * A signed change for the surface, in the units it is displayed in.
 *
 * `+0.0` and `-0.0` are both rendered as an unchanged `0.0`: a sign on a number that did
 * not move claims a direction the number does not have.
 */
export function formatChange(change: number, digits: number): string {
  const rounded = Number(change.toFixed(digits));
  if (rounded === 0) return (0).toFixed(digits);
  return `${rounded > 0 ? '+' : '−'}${Math.abs(rounded).toFixed(digits)}`;
}

/** A probability change as percentage points, which is how the headline is shown. */
export function formatProbabilityChange(change: number): string {
  return `${formatChange(change * 100, 1)} pp`;
}

/** Which way a change points, for the colour a surface gives it. */
export function changeDirection(change: number): 'up' | 'down' | 'none' {
  if (change > 0) return 'up';
  if (change < 0) return 'down';
  return 'none';
}
