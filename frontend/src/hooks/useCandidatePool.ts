import { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import type { CandidatesResponse } from '../types';

/**
 * One side's candidate list: what the pool would offer, what the retirement ledger is
 * keeping out, and the subset the user has ticked (D-12).
 *
 * The ticked subset lives here rather than in the dialog's markup because it survives the
 * dialog closing: a manual pool is a decision about the prediction, not about the window
 * that is open.
 */
export type CandidatePoolInput = {
  format: string;
  clubId: number | null;
  matchDate: string;
  /** The list to draw from: the recency window by default, all-time when true. */
  allTime: boolean;
  /** Whether to load at all. The dialog opening is what asks for the list. */
  enabled: boolean;
};

export function useCandidatePool({
  format,
  clubId,
  matchDate,
  allTime,
  enabled,
}: CandidatePoolInput) {
  const candidates = useAsync(api.getCandidates, {
    errorMessage: 'Failed to load the candidate list',
  });
  const flag = useAsync(api.flagRetirement, { errorMessage: 'Failed to flag the player' });
  const unflag = useAsync(api.unflagRetirement, { errorMessage: 'Failed to undo the exclusion' });

  const { run: loadCandidates, reset: resetCandidates } = candidates;

  const reload = useCallback(() => {
    if (!enabled || !format || !clubId) return;
    void loadCandidates({
      format,
      club_id: clubId,
      match_date: matchDate || undefined,
      all_time: allTime || undefined,
    });
  }, [enabled, format, clubId, matchDate, allTime, loadCandidates]);

  useEffect(() => {
    if (!enabled || !format || !clubId) {
      resetCandidates();
      return;
    }
    reload();
  }, [enabled, format, clubId, reload, resetCandidates]);

  // Flagging and un-flagging both change what the list says about a player, so the list
  // is re-read rather than patched in place: the answer to "is he excluded now" is the
  // ledger's, and a local guess at it is the kind of quiet divergence D-12 was.
  const { run: runFlag } = flag;
  const flagRetired = useCallback(
    async (playerId: number) => {
      const status = await runFlag(playerId, format || undefined);
      reload();
      return status;
    },
    [runFlag, format, reload],
  );

  const { run: runUnflag } = unflag;
  const undoExclusion = useCallback(
    async (playerId: number) => {
      const status = await runUnflag(playerId);
      reload();
      return status;
    },
    [runUnflag, reload],
  );

  return {
    data: candidates.data as CandidatesResponse | null,
    loading: candidates.loading,
    error: candidates.error ?? flag.error ?? unflag.error,
    working: flag.loading || unflag.loading,
    lastFlag: flag.data ?? null,
    flagRetired,
    undoExclusion,
    reload,
  };
}

/**
 * The ticked subset for one side.
 *
 * `null` means no manual pick — the default pool, unchanged flow — which is different
 * from an empty selection, and the difference is what keeps manual picking optional.
 */
export function useManualSelection() {
  const [selected, setSelected] = useState<number[] | null>(null);

  const toggle = useCallback((playerId: number, checked: boolean) => {
    setSelected((current) => {
      const next = new Set(current ?? []);
      if (checked) next.add(playerId);
      else next.delete(playerId);
      return [...next];
    });
  }, []);

  const clear = useCallback(() => setSelected(null), []);
  const selectAll = useCallback((playerIds: number[]) => setSelected(playerIds), []);

  return { selected, toggle, clear, selectAll };
}
