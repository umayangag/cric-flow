import { useCallback, useEffect, useMemo, useRef } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import { useBacktestFormOptions } from './useBacktestFormOptions';
import { useEvaluateCandidates } from './useEvaluateCandidates';
import { useEvaluateJob } from './useEvaluateJob';
import { useMatchScorecard } from './useMatchScorecard';
import { ApiError } from '../lib/apiError';
import type {
  BacktestCandidate,
  BacktestEvaluateResponse,
  MatchScorecardResponse,
  ModelStatsResponse,
} from '../types';

export type { EvaluationStep } from './useEvaluateJob';
import type { EvaluationStep } from './useEvaluateJob';

export interface UseEvaluateDbReturn {
  // Form inputs
  format: string;
  setFormat: (v: string) => void;
  team1: string;
  setTeam1: (v: string) => void;
  team2: string;
  setTeam2: (v: string) => void;

  // Options
  availableFormats: string[];
  availableTeam1s: string[];
  availableTeam2s: string[];

  // UI state
  loading: boolean;
  error: ApiError | null;
  statusMessage: string;

  // Backtest data
  candidates: BacktestCandidate[];
  selectedMatchId: number | null;
  setSelectedMatchId: (v: number | null) => void;
  evaluationResult: BacktestEvaluateResponse | null;

  // Scorecard
  scorecard: MatchScorecardResponse | null;
  scorecardLoading: boolean;
  scorecardError: ApiError | null;

  // Evaluate job
  currentJobId: string | null;
  evaluating: boolean;
  evaluationSteps: EvaluationStep[];

  /**
   * Provenance of the artifacts that answered the evaluation (W3-4).
   *
   * Loaded once with the tab rather than after a result: an operator deciding whether
   * to trust a number should not have to run an evaluation to find out the model was
   * trained on a dataset that is no longer here.
   */
  modelStats: ModelStatsResponse | null;
  modelStatsLoading: boolean;

  // Derived
  canLoad: boolean;
  canEvaluate: boolean;

  // Actions
  resetOutputs: () => void;
  handleLoadCandidates: () => Promise<void>;
  handleEvaluateSelectedMatch: () => Promise<void>;
}

/**
 * The Evaluate tab's state, composed from the three concerns it used to hold at once.
 *
 * It was 365 lines and 17 `useState` calls covering candidate loading, the evaluation
 * job, polling, the scorecard and a set of model-mode flags — plus a callback whose
 * job was keeping two copies of those flags in agreement. W0-2 removed the flags,
 * W3-1 split the rest into {@link useEvaluateCandidates}, {@link useEvaluateJob} and
 * {@link useMatchScorecard}, and this is what is left: wiring, and the two questions
 * the components actually ask — can I load, can I evaluate.
 *
 * The seams are the ones the data has. Selection belongs to the candidate list, not to
 * the job; the scorecard depends on the selected match and nothing else; the job knows
 * the parameters it was started with, so nothing has to be reconciled against the form
 * (W3-2 — the reconciliation callback is gone rather than smaller).
 */
export function useEvaluateDb(): UseEvaluateDbReturn {
  const {
    format,
    setFormat,
    team1,
    setTeam1,
    team2,
    setTeam2,
    availableFormats,
    availableTeam1s,
    availableTeam2s,
    optionsError,
    setOptionsError,
  } = useBacktestFormOptions();

  const modelStats = useAsync(api.getModelStats, { runOnMount: [] });
  const candidates = useEvaluateCandidates();
  const job = useEvaluateJob();
  const scorecard = useMatchScorecard(candidates.selectedMatchId);

  const { setSelectedMatchId, reload: reloadCandidates } = candidates;
  const { restore } = job;

  // Pick up a job left running before a refresh, and put the form back the way it was.
  // Once, on mount: this restores a snapshot, it does not subscribe to anything.
  const restored = useRef(false);
  useEffect(() => {
    if (restored.current) return;
    restored.current = true;
    void (async () => {
      const previous = await restore();
      if (!previous) return;
      setFormat(previous.format);
      setTeam1(previous.team1);
      setTeam2(previous.team2);
      setSelectedMatchId(previous.matchId);
      // The table has to show the match list again for the selected row to mean
      // anything. A failure here leaves the job visible and the table empty, which is
      // worse than it sounds only if it is silent — it is not, the list reports it.
      await reloadCandidates(previous.format, previous.team1, previous.team2);
    })();
  }, [restore, setFormat, setTeam1, setTeam2, setSelectedMatchId, reloadCandidates]);

  const loading = candidates.candidatesLoading;
  const error =
    job.error ?? candidates.candidatesError ?? (optionsError ? new ApiError(optionsError) : null);

  const canLoad = Boolean(format && team1 && team2) && !loading;
  const canEvaluate =
    Boolean(format && team1 && team2) &&
    candidates.selectedMatchId != null &&
    !loading &&
    !job.evaluating;

  const resetOutputs = useCallback(() => {
    candidates.clear();
    job.clear();
    setOptionsError(null);
  }, [candidates, job, setOptionsError]);

  const handleLoadCandidates = useCallback(async () => {
    setOptionsError(null);
    await candidates.load(format, team1, team2);
  }, [candidates, format, team1, team2, setOptionsError]);

  const handleEvaluateSelectedMatch = useCallback(async () => {
    if (candidates.selectedMatchId == null) return;
    await job.start(format, team1, team2, candidates.selectedMatchId);
  }, [job, format, team1, team2, candidates.selectedMatchId]);

  const statusMessage = useMemo(() => {
    if (job.statusMessage) return job.statusMessage;
    if (candidates.candidatesLoading) return 'Loading played matches…';
    if (candidates.candidatesError || !candidates.loaded) return '';
    return `Loaded ${candidates.candidates.length} candidates for ${team1} vs ${team2} (${format}).`;
  }, [job.statusMessage, candidates, team1, team2, format]);

  return {
    format,
    setFormat,
    team1,
    setTeam1,
    team2,
    setTeam2,
    availableFormats,
    availableTeam1s,
    availableTeam2s,
    loading,
    error,
    statusMessage,
    candidates: candidates.candidates,
    selectedMatchId: candidates.selectedMatchId,
    setSelectedMatchId: candidates.setSelectedMatchId,
    evaluationResult: job.result,
    scorecard: scorecard.scorecard,
    scorecardLoading: scorecard.scorecardLoading,
    scorecardError: scorecard.scorecardError,
    modelStats: modelStats.data,
    modelStatsLoading: modelStats.loading,
    currentJobId: job.jobId,
    evaluating: job.evaluating,
    evaluationSteps: job.steps,
    canLoad,
    canEvaluate,
    resetOutputs,
    handleLoadCandidates,
    handleEvaluateSelectedMatch,
  };
}
