import React, { useMemo, useState, useEffect, useCallback } from 'react';
import {
  Box,
  Button,
  FormControl,
  Grid,
  InputLabel,
  LinearProgress,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  MenuItem,
  Paper,
  Select,
  Stack,
  Typography,
  Alert,
  CircularProgress,
  TextField,
} from '@mui/material';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import Autocomplete, { createFilterOptions } from '@mui/material/Autocomplete';
import { api } from '../api';
import { accentGradient } from '../theme';
import { usePolling } from '../hooks/usePolling';
import type { BacktestCandidate, BacktestEvaluateResponse, MatchScorecardResponse } from '../types';
import CandidatesTable from './CandidatesTable';
import EvaluationResults from './EvaluationResults';
import MatchScorecard from './MatchScorecard';

const EVAL_JOB_STORAGE_KEY = 'cric_info_eval_job';

type StoredEvalJob = {
  job_id: string;
  match_id: number;
  format: string;
  team1: string;
  team2: string;
  started_at: string;
};

const filter = createFilterOptions<string>();

/** Shared filter for Team 1/Team 2 Autocomplete: show all when empty, require ≥3 chars when typing. */
function teamFilterOptions(options: string[], params: Parameters<typeof filter>[1]): string[] {
  const filtered = filter(options, params);
  if (params.inputValue === '') return filtered;
  if (params.inputValue.length < 3) return [];
  return filtered;
}

const EvaluateDbTab: React.FC = () => {
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
          // Don't auto-select format on initial load
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
          // Don't auto-select team1; keep it empty until user selects
          // Only clear if current selection is invalid (functional updater avoids stale closure)
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
    // team1 excluded: only used to validate/clear selection; API call depends only on format.
    // If effect logic evolves, prefer useRef or function setState to avoid stale closure bugs.
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
          // Don't auto-select team2; keep it empty until user selects
          // Only clear if current selection is invalid (functional updater avoids stale closure)
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
    // team2 excluded: only used to validate/clear selection; API call depends only on format+team1.
    // If effect logic evolves, prefer useRef or function setState to avoid stale closure bugs.
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
  const [evaluationSteps, setEvaluationSteps] = useState<
    Array<{ step: string; message: string }>
  >([]);

  const canLoad = useMemo(
    () => !!format && !!team1 && !!team2 && !loading,
    [format, team1, team2, loading],
  );
  const canEvaluate = useMemo(
    () => !!format && !!team1 && !!team2 && selectedMatchId != null && !loading && !evaluating,
    [format, team1, team2, selectedMatchId, loading, evaluating],
  );

  const resetOutputs = () => {
    setCandidates([]);
    setSelectedMatchId(null);
    setEvaluationResult(null);
    setScorecard(null);
    setScorecardError(null);
    setStatusMessage('');
    setError(null);
    setCurrentJobId(null);
    try {
      localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
    } catch {
      /* ignore */
    }
  };

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
  }, []);

  // Poll evaluate status while job is running
  const pollEvaluateStatus = useCallback(async () => {
    if (!currentJobId || !evaluating) return;
    try {
      const status = await api.getEvaluateStatus(currentJobId);
      setEvaluationSteps(
        status.steps?.map((s) => ({ step: s.step, message: s.message })) ?? [],
      );
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
  }, [currentJobId, evaluating]);

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

  const handleLoadCandidates = async () => {
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
  };

  const handleEvaluateSelectedMatch = async () => {
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
      setStatusMessage(
        'Evaluation in progress. You can refresh the page; progress will be restored.',
      );
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
      setStatusMessage('');
    }
  };

  return (
    <Stack spacing={3} sx={{ mt: 2 }}>
      <Typography variant="body1">
        Evaluate historical matches by predicting for actual players and comparing predictions vs
        actuals. Uses the latest loaded model (fast, good for QA).
      </Typography>
      <Alert severity="info" sx={{ maxWidth: 560 }}>
        Strict cutoff and train-on-the-fly are disabled to avoid high CPU/RAM usage. Pre-trained
        artifacts must be loaded for your format (e.g. T20I). Use Latest model only.
      </Alert>

      <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} alignItems="center">
        <FormControl fullWidth size="small">
          <InputLabel id="format-select-label">Format</InputLabel>
          <Select
            labelId="format-select-label"
            value={format}
            label="Format"
            onChange={(e) => {
              setFormat(e.target.value);
              resetOutputs();
            }}
          >
            <MenuItem value="">Select Format</MenuItem>
            {availableFormats.map((f) => (
              <MenuItem key={f} value={f}>
                {f}
              </MenuItem>
            ))}
          </Select>
        </FormControl>

        <Autocomplete
          fullWidth
          size="small"
          disableClearable
          options={(availableTeam1s || []).filter((t) => t !== team2)}
          value={(team1 || null) as string | undefined}
          onChange={(_e, newValue) => {
            setTeam1(newValue ?? '');
            if (newValue) resetOutputs();
          }}
          filterOptions={teamFilterOptions}
          renderInput={(params) => <TextField {...params} label="Team 1" />}
          noOptionsText={team1 ? 'No matching teams' : 'Type to search or select from dropdown'}
        />

        <Autocomplete
          fullWidth
          size="small"
          disableClearable
          options={(availableTeam2s || []).filter((t) => t !== team1)}
          value={(team2 || null) as string | undefined}
          onChange={(_e, newValue) => {
            setTeam2(newValue ?? '');
            if (newValue) resetOutputs();
          }}
          filterOptions={teamFilterOptions}
          renderInput={(params) => <TextField {...params} label="Team 2" />}
          noOptionsText={team2 ? 'No matching teams' : 'Type to search or select from dropdown'}
        />

        <Button
          variant="contained"
          onClick={handleLoadCandidates}
          disabled={!canLoad}
          sx={{ minWidth: 180, height: 40 }}
        >
          {loading && !candidates.length ? <CircularProgress size={20} sx={{ mr: 1 }} /> : null}
          Load Matches
        </Button>
      </Stack>

      {statusMessage && <Alert severity="info">{statusMessage}</Alert>}
      {error && <Alert severity="error">{error}</Alert>}

      {/* Current evaluation (restored after refresh or in progress) — so user sees which job and where the result is */}
      {currentJobId && (
        <Paper
          variant="outlined"
          sx={{
            p: 2,
            bgcolor: 'action.hover',
            borderColor: 'divider',
            borderWidth: 1,
          }}
          component="section"
          aria-label="current-evaluation"
        >
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Current evaluation
          </Typography>
          <Typography variant="body2">
            {format} — {team1} vs {team2}
            {selectedMatchId != null && ` · Match ${selectedMatchId}`}
            {evaluating && ' · Running (progress below)'}
            {evaluationResult && !evaluating && ' · Complete (result below)'}
            {error && !evaluating && ' · Failed (see error above)'}
          </Typography>
        </Paper>
      )}

      {/* Candidates */}
      <Box component="section" aria-label="candidates-section">
        <Typography variant="h6" gutterBottom>
          Candidates
        </Typography>
        <CandidatesTable
          candidates={candidates}
          selectedMatchId={selectedMatchId}
          onSelectMatch={setSelectedMatchId}
        />
        {selectedMatchId != null &&
          (scorecard ||
            scorecardLoading ||
            scorecardError ||
            evaluationResult?.predicted_scorecard) && (
            <Box sx={{ mt: 2 }}>
              <Typography variant="subtitle1" fontWeight="bold" gutterBottom>
                Scorecard: Actual vs Predicted
              </Typography>
              <Grid container spacing={3}>
                <Grid item xs={12} md={6}>
                  <MatchScorecard
                    scorecard={scorecard}
                    loading={scorecardLoading}
                    error={scorecardError}
                    title="Actual"
                    subtitle="Actual match result from database"
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <MatchScorecard
                    scorecard={evaluationResult?.predicted_scorecard ?? null}
                    loading={evaluating}
                    error={null}
                    title="Predicted"
                    subtitle={
                      evaluationResult
                        ? String(evaluationResult.filters.model_mode) === 'latest'
                          ? 'ML prediction using the latest model'
                          : 'ML prediction (model trained before match date)'
                        : 'Run evaluation to see predicted scorecard'
                    }
                  />
                </Grid>
              </Grid>
            </Box>
          )}
        <Box sx={{ mt: 2, display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', gap: 2 }}>
          <FormControl size="small" sx={{ minWidth: 260 }}>
            <InputLabel id="eval-db-temporal-label">Model temporal mode</InputLabel>
            <Select
              labelId="eval-db-temporal-label"
              value={useLatestModel ? 'latest' : 'strict'}
              label="Model temporal mode"
              onChange={(e) => setUseLatestModel(e.target.value === 'latest')}
            >
              <MenuItem value="latest">Latest model only</MenuItem>
              <MenuItem value="strict">Strict cutoff (requires train-on-the-fly)</MenuItem>
            </Select>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.5, maxWidth: 340 }}
            >
              Latest: pre-loaded artifacts. Strict: train-on-the-fly with data before match.
            </Typography>
          </FormControl>
          <FormControl size="small" sx={{ minWidth: 260 }}>
            <InputLabel id="eval-db-model-label">Prediction model</InputLabel>
            <Select
              labelId="eval-db-model-label"
              value={predictionModel}
              onChange={(e) => setPredictionModel(e.target.value as 'format' | 'unified')}
              label="Prediction model"
            >
              <MenuItem value="format">Format-specific (model for selected format)</MenuItem>
              <MenuItem value="unified">Unified (all-formats / legacy model)</MenuItem>
            </Select>
          </FormControl>
          <Button
            variant="contained"
            color="secondary"
            onClick={handleEvaluateSelectedMatch}
            disabled={!canEvaluate}
          >
            {evaluating ? (
              <>
                <CircularProgress size={20} sx={{ mr: 1 }} color="inherit" />
                Evaluating…
              </>
            ) : (
              'Evaluate Selected Match'
            )}
          </Button>
        </Box>

        {/* Status and error directly below button so user doesn't have to scroll up */}
        {selectedMatchId != null && (statusMessage || error) && (
          <Box sx={{ mt: 2 }}>
            {statusMessage && (
              <Alert severity="info" sx={{ mb: error ? 1 : 0 }}>
                {statusMessage}
              </Alert>
            )}
            {error && <Alert severity="error">{error}</Alert>}
          </Box>
        )}

        {/* Progress steps while evaluating (SSE stream) */}
        {evaluating && evaluationSteps.length > 0 && (
          <Paper
            variant="outlined"
            sx={{
              mt: 2,
              p: 2,
              pl: 2.5,
              position: 'relative',
              '&::before': {
                content: '""',
                position: 'absolute',
                left: 0,
                top: 0,
                bottom: 0,
                width: 4,
                background: accentGradient,
                borderRadius: '0 4px 4px 0',
              },
            }}
          >
            <Typography variant="subtitle2" fontWeight="bold" gutterBottom>
              Current step
            </Typography>
            <LinearProgress sx={{ mb: 2 }} />
            <List dense disablePadding>
              {evaluationSteps.map((s, idx) => (
                <ListItem key={`${s.step}-${idx}`} disablePadding sx={{ py: 0.25 }}>
                  <ListItemIcon sx={{ minWidth: 32 }}>
                    {idx === evaluationSteps.length - 1 ? (
                      <CircularProgress size={16} color="primary" />
                    ) : (
                      <CheckCircleIcon color="success" fontSize="small" />
                    )}
                  </ListItemIcon>
                  <ListItemText primary={s.message} primaryTypographyProps={{ variant: 'body2' }} />
                </ListItem>
              ))}
            </List>
          </Paper>
        )}
      </Box>

      {/* Results */}
      {evaluationResult && (
        <Box component="section" aria-label="results-section">
          <Typography variant="h6" gutterBottom sx={{ mt: 4 }}>
            Evaluation Results
          </Typography>
          <EvaluationResults result={evaluationResult} />
        </Box>
      )}
    </Stack>
  );
};

export default EvaluateDbTab;
