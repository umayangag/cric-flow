import { useCallback } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { AuctionProjection, AuctionResponse, AuctionVenueWeight } from '../types';

/**
 * The projection a candidate gets, and the assumptions it is made under (P3-2).
 *
 * As thin as `useAuction`, and for the same reason: every number on a projection is one
 * the backend computed from the two ml-service answers, and a range recomputed here would
 * be a second definition of an interval the surface is meant to be reporting. Nothing is
 * widened, narrowed or adjusted on this side — the ranges rule (P1-4) is the module's.
 *
 * There is no win probability to drop here either: go-app's own type for `/simulate`
 * carries no such field, so the value never reaches this process.
 */
export type AuctionProjectionRequest = {
  playerId: number;
  /** Undefined is the toss unknown, and the model marginalises over both batting orders. */
  team1BatsFirst?: boolean;
  /** Absent means no mixed range: how often an eleven plays where is nobody's fact here. */
  venueWeights?: AuctionVenueWeight[];
};

export function useAuctionProjection(
  auctionId: string | null,
  onRecord: (record: AuctionResponse) => void,
) {
  const projection = useAsync(api.projectAuctionCandidate, {
    errorMessage: 'Failed to project this candidate',
  });
  const assumptions = useAsync(api.setAuctionAssumptions, {
    errorMessage: 'Failed to save the assumptions',
  });
  const suggestion = useAsync(api.auctionOppositionSuggestion, {
    errorMessage: 'Failed to read that side’s last eleven',
  });

  const { run: runProjection, setData: setProjection } = projection;
  const project = useCallback(
    async (request: AuctionProjectionRequest) => {
      if (!auctionId) return null;
      return runProjection(auctionId, {
        player_id: request.playerId,
        team1_bats_first: request.team1BatsFirst,
        venue_weights: request.venueWeights,
      });
    },
    [auctionId, runProjection],
  );

  const { run: runAssumptions } = assumptions;
  const saveAssumptions = useCallback(
    async (body: {
      likely_xi?: number[];
      opposition?: { club_id: number; player_ids: number[] };
    }) => {
      if (!auctionId) return null;
      const written = await runAssumptions(auctionId, body);
      if (written) {
        onRecord(written);
        // The projection on screen was made under the assumptions that have just changed,
        // so it is cleared rather than left beside them: a range labelled with an eleven
        // it was not computed for is the substitution §8.7 forbids.
        setProjection(null);
      }
      return written;
    },
    [auctionId, runAssumptions, onRecord, setProjection],
  );

  const { run: runSuggestion } = suggestion;
  const suggestOpposition = useCallback(
    async (clubId: number) => {
      if (!auctionId) return null;
      return runSuggestion(auctionId, clubId);
    },
    [auctionId, runSuggestion],
  );

  return {
    projection: projection.data as AuctionProjection | null,
    projecting: projection.loading,
    projectionError: projection.error,
    project,
    clearProjection: useCallback(() => setProjection(null), [setProjection]),
    saveAssumptions,
    savingAssumptions: assumptions.loading,
    assumptionsError: assumptions.error,
    suggestion: suggestion.data,
    suggesting: suggestion.loading,
    suggestionError: suggestion.error,
    suggestOpposition,
  };
}
