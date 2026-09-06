import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { playDelta, type PlayDelta } from '../lib/playDelta';
import type { PredictTeamSelectionResponse } from '../types';

/**
 * Play mode: edit either eleven and see every change re-scored (P1-2).
 *
 * What it holds is the two elevens the user is building and the answer they were last
 * scored against. What it does *not* hold is a number: every value the surface shows comes
 * from a response, and the only arithmetic is {@link playDelta}'s subtraction of one
 * response's numbers from another's.
 *
 * A change that leaves a side short of a full eleven does not re-score. An eleven is the
 * unit every model here aggregates — the display model's, the objective's and the
 * simulator's alike — so a ten-man side would be a prediction for a match nobody plays,
 * and it would look like every other prediction. The surface says "add a player" instead.
 */

/** One player in a hand-built eleven: what the response carries about him. */
export type PlayPlayer = {
  player_id: number;
  player_name: string;
};

/** Which side an edit is on. 1 and 2 are the response's own two sides. */
export type PlaySide = 1 | 2;

/** Ask the API to score exactly these two elevens (P1-2's pinned path). */
export type RescoreXIs = (team1: number[], team2: number[]) => Promise<void>;

export type PlayModeInput = {
  /** The answer on screen, whether it was searched for or re-scored. */
  result: PredictTeamSelectionResponse | null;
  /** Score the two elevens the user has built. */
  rescore: RescoreXIs;
  /** Whether a request is in flight, so a delta is not shown against a pending answer. */
  loading: boolean;
};

function playersOf(result: PredictTeamSelectionResponse, side: PlaySide): PlayPlayer[] {
  const players = side === 1 ? result.team1 : result.team2;
  return players.map((player) => ({
    player_id: player.player_id,
    player_name: player.player_name,
  }));
}

/**
 * How many players an eleven holds here.
 *
 * It is read off the answer rather than assumed: `constraints.team_size` where Play mode
 * asked for a check, and otherwise the size of the eleven the API just returned. A
 * hardcoded 11 would be a second copy of a number the server already decides.
 */
export function teamSizeOf(result: PredictTeamSelectionResponse): number {
  return result.constraints?.team_size ?? result.team1.length;
}

export function usePlayMode({ result, rescore, loading }: PlayModeInput) {
  const [team1, setTeam1] = useState<PlayPlayer[]>([]);
  const [team2, setTeam2] = useState<PlayPlayer[]>([]);
  // The answer the next one is compared against: the state before the change the user
  // just made. Cleared by Optimise, because a searched eleven is a new baseline and not a
  // step away from the previous one.
  const [previous, setPrevious] = useState<PredictTeamSelectionResponse | null>(null);
  const [edits, setEdits] = useState(0);
  const scoredRef = useRef<PredictTeamSelectionResponse | null>(null);

  // A searched answer seeds the board and becomes the baseline; a re-scored one is the
  // answer to the board as it already stands, so it changes neither.
  useEffect(() => {
    scoredRef.current = result;
    if (!result || result.selection.objective === 'fixed') return;
    setTeam1(playersOf(result, 1));
    setTeam2(playersOf(result, 2));
    setPrevious(null);
    setEdits(0);
  }, [result]);

  const teamSize = result ? teamSizeOf(result) : 0;
  const complete = team1.length === teamSize && team2.length === teamSize && teamSize > 0;

  // A change re-scores as soon as both sides hold a full eleven, and the answer on screen
  // becomes what the next answer is measured against.
  const applyEdit = useCallback(
    (side: PlaySide, next: PlayPlayer[]) => {
      const nextTeam1 = side === 1 ? next : team1;
      const nextTeam2 = side === 2 ? next : team2;
      if (side === 1) setTeam1(next);
      else setTeam2(next);
      setEdits((count) => count + 1);
      if (nextTeam1.length !== teamSize || nextTeam2.length !== teamSize) return;
      setPrevious(scoredRef.current);
      void rescore(
        nextTeam1.map((player) => player.player_id),
        nextTeam2.map((player) => player.player_id),
      );
    },
    [team1, team2, teamSize, rescore],
  );

  const playersOn = useCallback((side: PlaySide) => (side === 1 ? team1 : team2), [team1, team2]);

  const addPlayer = useCallback(
    (side: PlaySide, player: PlayPlayer) => {
      const current = playersOn(side);
      if (current.some((held) => held.player_id === player.player_id)) return;
      applyEdit(side, [...current, player]);
    },
    [applyEdit, playersOn],
  );

  const removePlayer = useCallback(
    (side: PlaySide, playerId: number) => {
      applyEdit(
        side,
        playersOn(side).filter((held) => held.player_id !== playerId),
      );
    },
    [applyEdit, playersOn],
  );

  /**
   * Swap one player for another in one action, keeping his place in the order.
   *
   * It is a single edit rather than a remove and an add because a swap keeps the eleven
   * an eleven: the user asked one question — "what if he played instead of him?" — and
   * gets one answer to it.
   */
  const swapPlayer = useCallback(
    (side: PlaySide, outPlayerId: number, incoming: PlayPlayer) => {
      const current = playersOn(side);
      if (current.some((held) => held.player_id === incoming.player_id)) return;
      applyEdit(
        side,
        current.map((held) => (held.player_id === outPlayerId ? incoming : held)),
      );
    },
    [applyEdit, playersOn],
  );

  // The delta is only ever between two answers to the same fixture, and never while one
  // of them is still being fetched.
  const delta: PlayDelta | null = useMemo(
    () => (loading ? null : playDelta(previous, result)),
    [loading, previous, result],
  );

  return {
    team1,
    team2,
    teamSize,
    complete,
    edited: edits > 0,
    addPlayer,
    removePlayer,
    swapPlayer,
    delta,
    /** The ids on the board, so a picker can leave out the players already selected. */
    selectedIds: useMemo(
      () => ({
        1: team1.map((player) => player.player_id),
        2: team2.map((player) => player.player_id),
      }),
      [team1, team2],
    ),
  };
}

export type PlayModeState = ReturnType<typeof usePlayMode>;
