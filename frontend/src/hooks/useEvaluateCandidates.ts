import { useCallback, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { BacktestCandidate } from '../types';

/**
 * The played matches to choose from, and which one is chosen.
 *
 * The second of `useEvaluateDb`'s three concerns (W3-1). Selection lives here rather
 * than beside the job because it is a property of the *list*: clearing the list must
 * clear the selection, and when those were separate states one could outlive the other.
 */
export function useEvaluateCandidates() {
  const candidates = useAsync(
    (format: string, team1: string, team2: string) => api.backtestSelect(format, team1, team2),
    { errorMessage: 'Failed to load played matches' },
  );
  const [selectedMatchId, setSelectedMatchId] = useState<number | null>(null);

  const { run, reset } = candidates;

  const load = useCallback(
    async (format: string, team1: string, team2: string): Promise<number> => {
      setSelectedMatchId(null);
      const resp = await run(format.trim(), team1.trim(), team2.trim());
      return resp?.candidates?.length ?? 0;
    },
    [run],
  );

  /** Reload without disturbing the selection — used when restoring a job on mount. */
  const reload = useCallback(
    async (format: string, team1: string, team2: string) => {
      await run(format, team1, team2);
    },
    [run],
  );

  const clear = useCallback(() => {
    setSelectedMatchId(null);
    reset();
  }, [reset]);

  return {
    candidates: (candidates.data?.candidates ?? []) as BacktestCandidate[],
    /**
     * True once a search has returned, whatever it returned.
     *
     * Distinct from `candidates.length > 0`: "we looked and there are none" is a
     * different answer from "nobody has looked", and the tab says so rather than
     * rendering the same emptiness for both.
     */
    loaded: candidates.data != null,
    candidatesLoading: candidates.loading,
    candidatesError: candidates.error,
    selectedMatchId,
    setSelectedMatchId,
    load,
    reload,
    clear,
  };
}
