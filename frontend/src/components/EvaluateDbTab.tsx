import React, { useMemo, useState, useEffect } from 'react';
import {
  Box,
  Button,
  FormControl,
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
import type { BacktestCandidate, BacktestEvaluateResponse, MatchScorecardResponse } from '../types';
import CandidatesTable from './CandidatesTable';
import EvaluationResults from './EvaluationResults';
import MatchScorecard from './MatchScorecard';

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

  // Backtest data
  const [candidates, setCandidates] = useState<BacktestCandidate[]>([]);
  const [selectedMatchId, setSelectedMatchId] = useState<number | null>(null);
  const [evaluationResult, setEvaluationResult] = useState<BacktestEvaluateResponse | null>(null);

  // Match summary (scorecard) for selected match
  const [scorecard, setScorecard] = useState<MatchScorecardResponse | null>(null);
  const [scorecardLoading, setScorecardLoading] = useState<boolean>(false);
  const [scorecardError, setScorecardError] = useState<string | null>(null);

  // Evaluate with progress (SSE stream)
  const [evaluating, setEvaluating] = useState<boolean>(false);
  const [evaluationSteps, setEvaluationSteps] = useState<Array<{ step: string; message: string }>>(
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

  const resetOutputs = () => {
    setCandidates([]);
    setSelectedMatchId(null);
    setEvaluationResult(null);
    setScorecard(null);
    setScorecardError(null);
    setStatusMessage('');
    setError(null);
  };

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

  const handleEvaluateSelectedMatch = () => {
    if (selectedMatchId == null) return;
    setEvaluating(true);
    setEvaluationSteps([]);
    setError(null);
    setStatusMessage('Evaluation in progress…');
    api
      .backtestEvaluateStream(format.trim(), team1.trim(), team2.trim(), selectedMatchId, {
        onProgress: (step, message) => {
          setEvaluationSteps((prev) => [...prev, { step, message }]);
        },
        onResult: (result) => {
          setEvaluationResult(result);
          setEvaluating(false);
          setStatusMessage('Evaluation complete.');
        },
        onError: (err) => {
          setError(err.message);
          setEvaluating(false);
          setStatusMessage('');
        },
      })
      .then(() => {
        // Stream completed (onResult or onError already called)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err));
        setEvaluating(false);
      });
  };

  return (
    <Stack spacing={3} sx={{ mt: 2 }}>
      <Typography variant="body1">
        Evaluate historical matches by training strictly up to the match date, predicting for actual
        players, and comparing predictions vs actuals.
      </Typography>

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
        {selectedMatchId != null && (
          <MatchScorecard scorecard={scorecard} loading={scorecardLoading} error={scorecardError} />
        )}
        <Box sx={{ mt: 2 }}>
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

        {/* Predicted scorecard (ML, data before match date) — shown after evaluate */}
        {evaluationResult?.predicted_scorecard && (
          <MatchScorecard
            scorecard={evaluationResult.predicted_scorecard}
            title="Predicted scorecard"
            subtitle="ML prediction using only data before the match date (no actual match data used)."
          />
        )}
        </Box>
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
