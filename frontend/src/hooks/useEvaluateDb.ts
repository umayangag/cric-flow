import { useMemo, useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { getStoredEvalJob, setStoredEvalJob, clearStoredEvalJob } from '../lib/evaluateDbStorage';
import { useBacktestFormOptions } from './useBacktestFormOptions';
import { usePolling } from './usePolling';
import type { BacktestCandidate, BacktestEvaluateResponse, MatchScorecardResponse } from '../types';

export type EvaluationStep = { step: string; message: string };

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
  error: string | null;
  statusMessage: string;

  // Backtest data
  candidates: BacktestCandidate[];
  selectedMatchId: number | null;
  setSelectedMatchId: (v: number | null) => void;
  evaluationResult: BacktestEvaluateResponse | null;

  // Scorecard
  scorecard: MatchScorecardResponse | null;
  scorecardLoading: boolean;
  scorecardError: string | null;

  // Evaluate job
  currentJobId: string | null;
  evaluating: boolean;
  evaluationSteps: EvaluationStep[];

  // Derived
  canLoad: boolean;
  canEvaluate: boolean;

  // Actions
  resetOutputs: () => void;
  handleLoadCandidates: () => Promise<void>;
  handleEvaluateSelectedMatch: () => Promise<void>;
}

export function useEvaluateDb(): UseEvaluateDbReturn {
  const formOptions = useBacktestFormOptions();
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
  } = formOptions;

  // UI state (action errors; options load errors come from formOptions.optionsError)
  const [loading, setLoading] = useState<boolean>(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const error = optionsError ?? actionError;
  const setError = useCallback(
    (v: string | null) => {
      setActionError(v);
      if (v === null) setOptionsError(null);
    },
    [setOptionsError],
  );
  const [statusMessage, setStatusMessage] = useState<string>('');

  // Backtest data
  const [candidates, setCandidates] = useState<BacktestCandidate[]>([]);
  const [selectedMatchId, setSelectedMatchId] = useState<number | null>(null);
  const [evaluationResult, setEvaluationResult] = useState<BacktestEvaluateResponse | null>(null);

  // Match summary (scorecard) for selected match
  const [scorecard, setScorecard] = useState<MatchScorecardResponse | null>(null);
  const [scorecardLoading, setScorecardLoading] = useState<boolean>(false);
  const [scorecardError, setScorecardError] = useState<string | null>(null);

  // Evaluate job (survives refresh: job_id stored in localStorage, poll status)
  const [currentJobId, setCurrentJobId] = useState<string | null>(null);
  const [evaluating, setEvaluating] = useState<boolean>(false);
  const [evaluationSteps, setEvaluationSteps] = useState<EvaluationStep[]>([]);

  const canLoad = useMemo(
    () => !!format && !!team1 && !!team2 && !loading,
    [format, team1, team2, loading],
  );
  const canEvaluate = useMemo(
    () => !!format && !!team1 && !!team2 && selectedMatchId != null && !loading && !evaluating,
    [format, team1, team2, selectedMatchId, loading, evaluating],
  );

  const resetOutputs = useCallback(() => {
    setCandidates([]);
    setSelectedMatchId(null);
    setEvaluationResult(null);
    setScorecard(null);
    setScorecardError(null);
    setStatusMessage('');
    setError(null);
    setCurrentJobId(null);
    clearStoredEvalJob();
  }, [setError]);

  // Restore evaluation job on mount (e.g. after refresh) — poll if still running
  useEffect(() => {
    let cancelled = false;
    const data = getStoredEvalJob();
    if (!data) return;

    api
      .getEvaluateStatus(data.job_id)
      .then((status) => {
        if (cancelled) return;
        setCurrentJobId(data.job_id);
        setFormat(data.format);
        setTeam1(data.team1);
        setTeam2(data.team2);
        setSelectedMatchId(data.match_id);
        setEvaluationSteps(status.steps?.map((s) => ({ step: s.step, message: s.message })) ?? []);
        if (status.status === 'running') {
          setEvaluating(true);
          setStatusMessage('Evaluation in progress (restored).');
        } else if (status.status === 'done' && status.result) {
          setEvaluating(false);
          setEvaluationResult(status.result);
          setStatusMessage('Evaluation complete.');
        } else if (status.status === 'error') {
          setEvaluating(false);
          setError(status.error ?? 'Unknown error');
          setStatusMessage('');
        }
        // Reload candidates so the table shows the match list and selected row after refresh
        api
          .backtestSelect(data.format, data.team1, data.team2)
          .then((resp) => {
            if (!cancelled) setCandidates(resp.candidates ?? []);
          })
          .catch(() => {
            /* ignore */
          });
      })
      .catch(() => {
        if (cancelled) return;
        clearStoredEvalJob();
      });
    return () => {
      cancelled = true;
    };
  }, [setFormat, setTeam1, setTeam2, setSelectedMatchId, setError]);

  // Poll evaluate status while job is running
  const pollEvaluateStatus = useCallback(async () => {
    if (!currentJobId || !evaluating) return;
    try {
      const status = await api.getEvaluateStatus(currentJobId);
      setEvaluationSteps(status.steps?.map((s) => ({ step: s.step, message: s.message })) ?? []);
      if (status.status === 'done') {
        setEvaluating(false);
        setEvaluationResult(status.result ?? null);
        setStatusMessage('Evaluation complete.');
        clearStoredEvalJob();
      } else if (status.status === 'error') {
        setEvaluating(false);
        setError(status.error ?? 'Unknown error');
        setStatusMessage('');
        clearStoredEvalJob();
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setEvaluating(false);
      if (msg.includes('404') || msg.includes('NOT_FOUND')) {
        setError('Evaluation job no longer available (server may have restarted).');
      } else {
        setError(msg || 'Failed to fetch evaluation status.');
      }
      setStatusMessage('');
      clearStoredEvalJob();
    }
  }, [currentJobId, evaluating, setError]);

  usePolling(pollEvaluateStatus, 2000, Boolean(currentJobId && evaluating));

  // Load match scorecard when a match is selected
  useEffect(() => {
    if (selectedMatchId == null) {
      setScorecard(null);
      setScorecardError(null);
      setScorecardLoading(false);
      return;
    }
    let active = true;
    setScorecardLoading(true);
    setScorecardError(null);
    api
      .getMatchScorecard(selectedMatchId)
      .then((data) => {
        if (active) {
          setScorecard(data);
          setScorecardError(null);
        }
      })
      .catch((e: unknown) => {
        if (active) {
          setScorecard(null);
          setScorecardError(e instanceof Error ? e.message : String(e));
        }
      })
      .finally(() => {
        if (active) setScorecardLoading(false);
      });
    return () => {
      active = false;
    };
  }, [selectedMatchId]);

  const handleLoadCandidates = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      setEvaluationResult(null);
      setStatusMessage('Loading played matches…');
      const resp = await api.backtestSelect(format, team1.trim(), team2.trim());
      setCandidates(resp.candidates || []);
      setStatusMessage(
        `Loaded ${resp.candidates?.length ?? 0} candidates for ${team1} vs ${team2} (${format}).`,
      );
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setStatusMessage('');
    } finally {
      setLoading(false);
    }
  }, [format, team1, team2, setError]);

  const handleEvaluateSelectedMatch = useCallback(async () => {
    if (selectedMatchId == null) return;
    setError(null);
    setStatusMessage('Starting evaluation…');
    try {
      const { job_id } = await api.evaluateStart(
        format.trim(),
        team1.trim(),
        team2.trim(),
        selectedMatchId,
      );
      setStoredEvalJob({
        job_id,
        match_id: selectedMatchId,
        format: format.trim(),
        team1: team1.trim(),
        team2: team2.trim(),
        started_at: new Date().toISOString(),
      });
      setCurrentJobId(job_id);
      setEvaluating(true);
      setEvaluationSteps([]);
      setStatusMessage(
        'Evaluation in progress. You can refresh the page; progress will be restored.',
      );
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
      setStatusMessage('');
    }
  }, [format, team1, team2, selectedMatchId, setError]);

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
    candidates,
    selectedMatchId,
    setSelectedMatchId,
    evaluationResult,
    scorecard,
    scorecardLoading,
    scorecardError,
    currentJobId,
    evaluating,
    evaluationSteps,
    canLoad,
    canEvaluate,
    resetOutputs,
    handleLoadCandidates,
    handleEvaluateSelectedMatch,
  };
}
