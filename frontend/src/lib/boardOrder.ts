import type { PredictSelectionSummary, PredictTeamSelectedPlayer } from '../types';

/**
 * The order an eleven is shown in, and the sentence that says what that order is (P1-4,
 * the other half of docs/BUG_BACKLOG.md § B-8).
 *
 * The Lab used to list every eleven in the order the response carried, which on a searched
 * format is the search's final order and on a rating-ordered format is the rating order —
 * and said nothing about either. B-8 found that the rating order and the display model
 * disagree about who is better, so a board that looks ranked but is not invites a swap the
 * number then punishes. The fix on the surface is to rank the board by an ordering it can
 * stand behind where one exists, and to say which ordering it is showing everywhere.
 *
 * Nothing here computes a number: a searched eleven is sorted by the marginal value the
 * response already carries, and every other eleven is left exactly as served.
 */

/** The players in the order the board shows them. */
export function boardOrder(
  players: PredictTeamSelectedPlayer[],
  selection: PredictSelectionSummary,
): PredictTeamSelectedPlayer[] {
  if (!selection.optimised) return players;
  // A stable sort, so two players with the same marginal value keep the served order and a
  // player with none (which a searched eleven should never carry) falls to the bottom.
  return [...players].sort(
    (a, b) =>
      (b.marginal_value ?? Number.NEGATIVE_INFINITY) -
      (a.marginal_value ?? Number.NEGATIVE_INFINITY),
  );
}

/** What the order on the board is, said so a reader does not take it for something else. */
export function boardOrderSentence(selection: PredictSelectionSummary): string {
  if (selection.optimised) {
    return "Ordered by marginal value: the objective's own ranking of this eleven, which is what a swap here is measured against.";
  }
  if (selection.objective === 'fixed') {
    return 'In the order you built it. Nothing ranks these players: no search ran, and no marginal value exists.';
  }
  return "In rating order: the selection's own composite, which is not the win model's ranking. The two disagree about who is better, so a swap for a higher-rated player can lower the displayed probability.";
}
