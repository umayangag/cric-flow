import { useMemo, useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { usePolling } from './usePolling';
import type { BacktestCandidate, BacktestEvaluateResponse, MatchScorecardResponse } from '../types';

const EVAL_JOB_STORAGE_KEY = 'cric_info_eval_job';

type StoredEvalJob = {
  job_id: string;
  match_id: number;
  format: string;
  team1: string;
  team2: string;
  started_at: string;
};

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

  // Model settings
  predictionModel: 'format' | 'unified';
  setPredictionModel: (v: 'format' | 'unified') => void;
  useLatestModel: boolean;
  setUseLatestModel: (v: boolean) => void;

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
  jobUseUnifiedModel: boolean | null;
  jobUseLatestModel: boolean | null;
}

export function useEvaluateDb(): UseEvaluateDbReturn {
  // Inputs for new backtest flow
  const [format, setFormat] = useState<string>('');
  const [team1, setTeam1] = useState<string>('');
  const [team2, setTeam2] = useState<string>('');

  // Options
  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [availableTeam1s, setAvailableTeam1s] = useState<string[]>([]);
  const [availableTeam2s, setAvailableTeam2s] = useState<string[]>([]);

  useEffect(() => {
    let active = true;
    const fetchData = async () => {
      try {
        const f = await api.getFormats();
        if (active) {
          setAvailableFormats(f);
        }
      } catch (e) {
        if (active) {
          setError(`Failed to load form options: ${e instanceof Error ? e.message : String(e)}`);
          console.error('Failed to fetch options', e);
        }
      }
    };
    fetchData();
    return () => {
      active = false;
    };
  }, []);

  // Fetch Team 1 when format changes
  useEffect(() => {
    if (!format) {
      setAvailableTeam1s([]);
      setTeam1('');
      return;
    }
    let active = true;
    api
      .getTeamsByFormat(format)
      .then((teams) => {
        if (active) {
          setAvailableTeam1s(teams);
          setTeam1((currentTeam1) =>
            currentTeam1 && !teams.includes(currentTeam1) ? '' : currentTeam1,
          );
        }
      })
      .catch((err) => {
        if (active) {
          setError(`Failed to fetch teams: ${err.message}`);
        }
      });
    return () => {
      active = false;
    };
    // team1 excluded: only used to validate/clear selection via functional updater;
    // API call depends only on format.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [format]);

  // Fetch Team 2 when Team 1 or format changes
  useEffect(() => {
    if (!format || !team1) {
      setAvailableTeam2s([]);
      setTeam2('');
      return;
    }
    let active = true;
    api
      .getOpponents(format, team1)
      .then((opps) => {
        if (active) {
          setAvailableTeam2s(opps);
          setTeam2((currentTeam2) =>
            currentTeam2 && !opps.includes(currentTeam2) ? '' : currentTeam2,
          );
        }
      })
      .catch((err) => {
        if (active) {
          setError(`Failed to fetch opponents: ${err.message}`);
        }
      });
    return () => {
      active = false;
    };
    // team2 excluded: only used to validate/clear selection via functional updater;
    // API call depends only on format+team1.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [format, team1]);

  // UI state
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string>('');

  // Prediction model for evaluate: format-specific or unified (legacy)
  const [predictionModel, setPredictionModel] = useState<'format' | 'unified'>('format');
  // Model temporal mode: strict (trained only on data before match) or latest (current model; may include match)
  const [useLatestModel, setUseLatestModel] = useState<boolean>(true);

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
  const [jobUseUnifiedModel, setJobUseUnifiedModel] = useState<boolean | null>(null);
  const [jobUseLatestModel, setJobUseLatestModel] = useState<boolean | null>(null);

  const applyJobModeFromStatus = useCallback(
    (status: { use_unified_model?: boolean; use_latest_model?: boolean }) => {
      setJobUseUnifiedModel(
        typeof status.use_unified_model === 'boolean' ? status.use_unified_model : null,
      );
      setJobUseLatestModel(
        typeof status.use_latest_model === 'boolean' ? status.use_latest_model : null,
      );
    },
    [],
  );

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
    setJobUseUnifiedModel(null);
    setJobUseLatestModel(null);
    try {
      localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
    } catch {
      /* ignore */
    }
  }, []);

  // Restore evaluation job on mount (e.g. after refresh) — poll if still running
  useEffect(() => {
    let cancelled = false;
    let stored: string | null = null;
    try {
      if (typeof localStorage?.getItem === 'function') {
        stored = localStorage.getItem(EVAL_JOB_STORAGE_KEY);
      }
    } catch {
      /* ignore (e.g. test env) */
    }
    if (!stored) return;
    let data: StoredEvalJob;
    try {
      data = JSON.parse(stored) as StoredEvalJob;
    } catch {
      localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
      return;
    }
    if (!data.job_id) return;

    api
      .getEvaluateStatus(data.job_id)
      .then((status) => {
        if (cancelled) return;
        setCurrentJobId(data.job_id);
        setFormat(data.format);
        setTeam1(data.team1);
        setTeam2(data.team2);
        setSelectedMatchId(data.match_id);
        applyJobModeFromStatus(status);
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
        localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
      });
    return () => {
      cancelled = true;
    };
  }, [applyJobModeFromStatus]);

  // Poll evaluate status while job is running
  const pollEvaluateStatus = useCallback(async () => {
    if (!currentJobId || !evaluating) return;
    try {
      const status = await api.getEvaluateStatus(currentJobId);
      setEvaluationSteps(status.steps?.map((s) => ({ step: s.step, message: s.message })) ?? []);
      applyJobModeFromStatus(status);
      if (status.status === 'done') {
        setEvaluating(false);
        setEvaluationResult(status.result ?? null);
        setStatusMessage('Evaluation complete.');
        try {
          localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
        } catch {
          /* ignore */
        }
      } else if (status.status === 'error') {
        setEvaluating(false);
        setError(status.error ?? 'Unknown error');
        setStatusMessage('');
        try {
          localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
        } catch {
          /* ignore */
        }
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
      try {
        localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
      } catch {
        /* ignore */
      }
    }
  }, [currentJobId, evaluating, applyJobModeFromStatus]);

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
  }, [format, team1, team2]);

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
        {
          use_unified_model: predictionModel === 'unified',
          use_latest_model: useLatestModel,
        },
      );
      const stored: StoredEvalJob = {
        job_id,
        match_id: selectedMatchId,
        format: format.trim(),
        team1: team1.trim(),
        team2: team2.trim(),
        started_at: new Date().toISOString(),
      };
      try {
        localStorage.setItem(EVAL_JOB_STORAGE_KEY, JSON.stringify(stored));
      } catch {
        /* ignore */
      }
      setCurrentJobId(job_id);
      setEvaluating(true);
      setEvaluationSteps([]);
      applyJobModeFromStatus({
        use_unified_model: predictionModel === 'unified',
        use_latest_model: useLatestModel,
      });
      setStatusMessage(
        'Evaluation in progress. You can refresh the page; progress will be restored.',
      );
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
      setStatusMessage('');
    }
  }, [
    format,
    team1,
    team2,
    selectedMatchId,
    predictionModel,
    useLatestModel,
    applyJobModeFromStatus,
  ]);

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
    predictionModel,
    setPredictionModel,
    useLatestModel,
    setUseLatestModel,
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
    jobUseUnifiedModel,
    jobUseLatestModel,
  };
}
