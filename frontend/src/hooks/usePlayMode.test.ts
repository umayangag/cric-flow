import { describe, it, expect, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { usePlayMode, teamSizeOf } from './usePlayMode';
import type { PredictTeamSelectionResponse, PredictTeamSelectedPlayer } from '../types';

/**
 * Play mode's state machine (P1-2): what an edit does, when it re-scores, and what the
 * delta is measured against.
 */

const pool = {
  source: 'recency_window' as const,
  window_months: 9,
  since: '2025-12-10',
  size: 24,
  retired_excluded: 0,
};

function player(id: number): PredictTeamSelectedPlayer {
  return { player_id: id, player_name: `Player ${id}`, runs: 30, wickets: 1, runs_conceded: 20 };
}

function answer(
  overrides: Partial<PredictTeamSelectionResponse> = {},
): PredictTeamSelectionResponse {
  return {
    ratings_through: '2026-09-02',
    run_id: '20260906T083819Z-36689f80',
    team1_side: { club_id: 43, name: 'India', gender: 'male', display_name: 'India (men)' },
    team2_side: { club_id: 7, name: 'Australia', gender: 'male', display_name: 'Australia (men)' },
    team1: [player(1), player(2)],
    team2: [player(3), player(4)],
    selection: { objective: 'win', optimised: true },
    record: { stored: true, id: 'f0f8f1a4-0f0e-4a6b-9b6f-2c5d4a1e0002' },
    forecast: { source: 'simulator' },
    win_probability: { team1: 0.6, source: 'display', predicted_winner: 'India (men)' },
    toss: { team1_bats_first: null, honoured: true },
    team1_pool: pool,
    team2_pool: pool,
    ...overrides,
  };
}

function playHook(result: PredictTeamSelectionResponse | null, rescore = vi.fn()) {
  const hook = renderHook(
    ({ current }: { current: PredictTeamSelectionResponse | null }) =>
      usePlayMode({ result: current, rescore, loading: false }),
    { initialProps: { current: result } },
  );
  return { ...hook, rescore };
}

describe('usePlayMode', () => {
  it('seeds both elevens from a searched answer', () => {
    const { result } = playHook(answer());

    expect(result.current.team1.map((p) => p.player_id)).toEqual([1, 2]);
    expect(result.current.team2.map((p) => p.player_id)).toEqual([3, 4]);
    expect(result.current.teamSize).toBe(2);
    expect(result.current.complete).toBe(true);
    expect(result.current.edited).toBe(false);
  });

  it('swaps one player for another in his place and re-scores', () => {
    const { result, rescore } = playHook(answer());

    act(() => result.current.swapPlayer(1, 2, { player_id: 9, player_name: 'Player 9' }));

    expect(result.current.team1.map((p) => p.player_id)).toEqual([1, 9]);
    expect(rescore).toHaveBeenCalledWith([1, 9], [3, 4]);
    expect(result.current.edited).toBe(true);
  });

  it('removes a player without re-scoring, because an eleven is scored as an eleven', () => {
    const { result, rescore } = playHook(answer());

    act(() => result.current.removePlayer(2, 3));

    expect(result.current.team2.map((p) => p.player_id)).toEqual([4]);
    expect(result.current.complete).toBe(false);
    expect(rescore).not.toHaveBeenCalled();
  });

  it('re-scores as soon as an addition completes the side again', () => {
    const { result, rescore } = playHook(answer());

    act(() => result.current.removePlayer(2, 3));
    act(() => result.current.addPlayer(2, { player_id: 8, player_name: 'Player 8' }));

    expect(result.current.team2.map((p) => p.player_id)).toEqual([4, 8]);
    expect(result.current.complete).toBe(true);
    expect(rescore).toHaveBeenCalledTimes(1);
    expect(rescore).toHaveBeenCalledWith([1, 2], [4, 8]);
  });

  it('ignores a player the side already holds, on both an add and a swap', () => {
    const { result, rescore } = playHook(answer());

    act(() => result.current.addPlayer(1, { player_id: 1, player_name: 'Player 1' }));
    act(() => result.current.swapPlayer(1, 2, { player_id: 1, player_name: 'Player 1' }));

    expect(result.current.team1.map((p) => p.player_id)).toEqual([1, 2]);
    expect(rescore).not.toHaveBeenCalled();
  });

  it('measures the delta against the answer the change was made from', () => {
    const rescore = vi.fn();
    const { result, rerender } = playHook(answer(), rescore);

    act(() => result.current.swapPlayer(1, 2, { player_id: 9, player_name: 'Player 9' }));
    rerender({
      current: answer({
        selection: { objective: 'fixed', optimised: false },
        team1: [player(1), player(9)],
        win_probability: { team1: 0.64, source: 'display', predicted_winner: 'India (men)' },
      }),
    });

    expect(result.current.delta?.winProbability.change).toBeCloseTo(0.04, 10);
    expect(result.current.team1.map((p) => p.player_id)).toEqual(
      [1, 9],
      // A re-scored answer answers the board as it already stands; it does not reseed it.
    );
  });

  it('starts again from a searched answer, with nothing to compare against', () => {
    const rescore = vi.fn();
    const { result, rerender } = playHook(answer(), rescore);

    act(() => result.current.swapPlayer(1, 2, { player_id: 9, player_name: 'Player 9' }));
    rerender({ current: answer({ team1: [player(5), player(6)] }) });

    expect(result.current.team1.map((p) => p.player_id)).toEqual([5, 6]);
    expect(result.current.delta).toBeNull();
    expect(result.current.edited).toBe(false);
  });

  it('holds nothing at all until there is an answer', () => {
    const { result } = playHook(null);

    expect(result.current.team1).toEqual([]);
    expect(result.current.teamSize).toBe(0);
    expect(result.current.complete).toBe(false);
  });

  it('shows no delta while an answer is being fetched', () => {
    const previous = answer();
    const { result } = renderHook(() =>
      usePlayMode({ result: previous, rescore: vi.fn(), loading: true }),
    );

    expect(result.current.delta).toBeNull();
  });
});

describe('teamSizeOf', () => {
  it('takes the size the constraint check reports where there is one', () => {
    expect(
      teamSizeOf(
        answer({
          constraints: {
            team_size: 11,
            min_bowlers: 5,
            require_keeper: true,
            team1: { size: 11, bowlers: 5, has_keeper: true, met: true },
            team2: { size: 11, bowlers: 5, has_keeper: true, met: true },
          },
        }),
      ),
    ).toBe(11);
  });

  it('otherwise takes the size of the eleven that was returned', () => {
    expect(teamSizeOf(answer())).toBe(2);
  });
});
