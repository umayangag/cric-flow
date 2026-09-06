import { describe, it, expect } from 'vitest';
import { boardOrder, boardOrderSentence } from './boardOrder';
import type { PredictSelectionSummary, PredictTeamSelectedPlayer } from '../types';

function player(id: number, marginal?: number): PredictTeamSelectedPlayer {
  return {
    player_id: id,
    player_name: `Player ${id}`,
    runs: 30,
    wickets: 1,
    runs_conceded: 20,
    marginal_value: marginal,
  };
}

const searched: PredictSelectionSummary = { objective: 'win', optimised: true };
const ratingOrdered: PredictSelectionSummary = { objective: 'ratings', optimised: false };
const handBuilt: PredictSelectionSummary = { objective: 'fixed', optimised: false };

describe('boardOrder', () => {
  // B-8's other half: on a searched eleven the board is ranked by the ordering the surface
  // can stand behind, which is the marginal value the response already carries.
  it('ranks a searched eleven by marginal value, highest first', () => {
    const served = [player(1, 0.01), player(2, 0.05), player(3, 0.03)];

    const ordered = boardOrder(served, searched);

    expect(ordered.map((p) => p.player_id)).toEqual([2, 3, 1]);
    expect(served.map((p) => p.player_id)).toEqual([1, 2, 3]);
  });

  it('keeps the served order between equal marginal values and puts a missing one last', () => {
    const served = [player(1), player(2, 0.02), player(3, 0.02)];

    const ordered = boardOrder(served, searched);

    expect(ordered.map((p) => p.player_id)).toEqual([2, 3, 1]);
  });

  it('leaves a rating-ordered eleven exactly as served', () => {
    const served = [player(1, 0.01), player(2, 0.05)];

    expect(boardOrder(served, ratingOrdered)).toBe(served);
  });

  it('leaves a hand-built eleven exactly as served', () => {
    const served = [player(2), player(1)];

    expect(boardOrder(served, handBuilt)).toBe(served);
  });
});

describe('boardOrderSentence', () => {
  it('says what each ordering is, so none is taken for the win model’s ranking', () => {
    expect(boardOrderSentence(searched)).toMatch(/ordered by marginal value/i);
    expect(boardOrderSentence(ratingOrdered)).toMatch(/not the win model's ranking/);
    expect(boardOrderSentence(handBuilt)).toMatch(/in the order you built it/i);
  });
});
