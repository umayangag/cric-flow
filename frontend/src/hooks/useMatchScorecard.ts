import { useEffect } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { MatchScorecardResponse } from '../types';

/**
 * The scorecard for the selected match, or nothing when no match is selected.
 *
 * One of the three concerns `useEvaluateDb` used to hold at once (W3-1). It is the
 * smallest of them and the least entangled: it depends on a match id and nothing else.
 */
export function useMatchScorecard(matchId: number | null) {
  const scorecard = useAsync((id: number) => api.getMatchScorecard(id), {
    errorMessage: 'Failed to load the match scorecard',
  });

  const { run, reset } = scorecard;
  useEffect(() => {
    if (matchId == null) {
      reset();
      return;
    }
    void run(matchId);
  }, [matchId, run, reset]);

  return {
    scorecard: scorecard.data as MatchScorecardResponse | null,
    scorecardLoading: scorecard.loading,
    scorecardError: scorecard.error,
  };
}
