import { useCallback, useState } from 'react';
import { api } from '../api';
import { getStoredEvalJob, setStoredEvalJob, clearStoredEvalJob } from '../lib/evaluateDbStorage';
import { usePolling } from './usePolling';
import { ApiError, toApiError } from '../lib/apiError';
import type { BacktestEvaluateResponse, EvaluateStatusResponse } from '../types';

/** One progress step reported by a running evaluation. */
export type EvaluationStep = { step: string; message: string };

/** How often a running job is asked for its progress. Paused while the tab is hidden. */
const POLL_MS = 2000;

/** What a restored job says about the form it was started from. */
export type RestoredJob = {
  format: string;
  team1: string;
  team2: string;
  matchId: number;
};

/**
 * The evaluation job: starting one, following it, and picking one up again after a
 * refresh.
 *
 * The third of `useEvaluateDb`'s concerns (W3-1), and the one that made the hook hard
 * to read: it held the job *and* the form the job was started from, kept in sync by a
 * reconciliation callback. There is nothing to reconcile now (W3-2) — a job carries
 * the parameters it was started with, in `localStorage`, and the form is somebody
 * else's state.
 */
export function useEvaluateJob() {
  const [jobId, setJobId] = useState<string | null>(null);
  const [evaluating, setEvaluating] = useState(false);
  const [steps, setSteps] = useState<EvaluationStep[]>([]);
  const [result, setResult] = useState<BacktestEvaluateResponse | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [statusMessage, setStatusMessage] = useState('');

  const clear = useCallback(() => {
    setJobId(null);
    setEvaluating(false);
    setSteps([]);
    setResult(null);
    setError(null);
    setStatusMessage('');
    clearStoredEvalJob();
  }, []);

  /** Fold one status response into state. Shared by the poll and the restore. */
  const applyStatus = useCallback((status: EvaluateStatusResponse) => {
    setSteps(status.steps?.map((s) => ({ step: s.step, message: s.message })) ?? []);
    switch (status.status) {
      case 'running':
        setEvaluating(true);
        return;
      case 'done':
        setEvaluating(false);
        setResult(status.result ?? null);
        setStatusMessage('Evaluation complete.');
        clearStoredEvalJob();
        return;
      case 'error':
        setEvaluating(false);
        setError(new ApiError(status.error ?? 'The evaluation failed without saying why.'));
        setStatusMessage('');
        clearStoredEvalJob();
    }
  }, []);

  /**
   * Pick up a job recorded before a refresh, and report the form it was started from
   * so the caller can restore it.
   *
   * Returns null when there is nothing to restore, or when the server no longer knows
   * the job — a stored id the server has forgotten is stale, and quietly dropping it
   * is right here, unlike mid-run, where it means the server restarted and the user
   * should be told.
   */
  const restore = useCallback(async (): Promise<RestoredJob | null> => {
    const stored = getStoredEvalJob();
    if (!stored) return null;
    try {
      const status = await api.getEvaluateStatus(stored.job_id);
      setJobId(stored.job_id);
      applyStatus(status);
      if (status.status === 'running') {
        setStatusMessage('Evaluation in progress (restored).');
      }
      return {
        format: stored.format,
        team1: stored.team1,
        team2: stored.team2,
        matchId: stored.match_id,
      };
    } catch {
      clearStoredEvalJob();
      return null;
    }
  }, [applyStatus]);

  const start = useCallback(
    async (format: string, team1: string, team2: string, matchId: number) => {
      setError(null);
      setResult(null);
      setSteps([]);
      setStatusMessage('Starting evaluation…');
      try {
        const { job_id } = await api.evaluateStart(
          format.trim(),
          team1.trim(),
          team2.trim(),
          matchId,
        );
        setStoredEvalJob({
          job_id,
          match_id: matchId,
          format: format.trim(),
          team1: team1.trim(),
          team2: team2.trim(),
          started_at: new Date().toISOString(),
        });
        setJobId(job_id);
        setEvaluating(true);
        setStatusMessage(
          'Evaluation in progress. You can refresh the page; progress will be restored.',
        );
      } catch (thrown) {
        setError(toApiError(thrown, 'Could not start the evaluation'));
        setStatusMessage('');
      }
    },
    [],
  );

  const poll = useCallback(async () => {
    if (!jobId || !evaluating) return;
    try {
      applyStatus(await api.getEvaluateStatus(jobId));
    } catch (thrown) {
      const failure = toApiError(thrown, 'Failed to fetch evaluation status.');
      setEvaluating(false);
      setStatusMessage('');
      clearStoredEvalJob();
      // A 404 mid-run means the job is gone, not that the request failed — jobs live
      // in memory, so a server restart loses them. Saying which one happened is the
      // difference between "try again" and "wait and refresh".
      setError(
        failure.status === 404 || failure.message.includes('NOT_FOUND')
          ? new ApiError('Evaluation job no longer available (the server may have restarted).', {
              hint: 'Start the evaluation again.',
            })
          : failure,
      );
    }
  }, [jobId, evaluating, applyStatus]);

  usePolling(poll, POLL_MS, Boolean(jobId && evaluating));

  return { jobId, evaluating, steps, result, error, statusMessage, start, restore, clear };
}
