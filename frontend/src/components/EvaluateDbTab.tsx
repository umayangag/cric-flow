import React, { useMemo, useState, useEffect } from 'react';
import {
  Box,
  Button,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Typography,
  Alert,
  CircularProgress,
  TextField,
} from '@mui/material';
import Autocomplete, { createFilterOptions } from '@mui/material/Autocomplete';
import { api } from '../api';
import type { BacktestCandidate, BacktestEvaluateResponse } from '../types';
import CandidatesTable from './CandidatesTable';
import EvaluationResults from './EvaluationResults';

const filter = createFilterOptions<string>();

const EvaluateDbTab: React.FC = () => {
  // Inputs for new backtest flow
  const [format, setFormat] = useState<string>('T20');
  const [team1, setTeam1] = useState<string>('IND');
  const [team2, setTeam2] = useState<string>('AUS');

  // Options
  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [availableTeams, setAvailableTeams] = useState<string[]>([]);

  useEffect(() => {
    let active = true;
    const fetchData = async () => {
      try {
        const [f, t] = await Promise.all([api.getFormats(), api.getTeams()]);
        if (active) {
          setAvailableFormats(f);
          setAvailableTeams(t);
        }
        // Optionally set defaults if current selection is invalid, but keeping it simple for now
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

  // UI state
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<string>('');

  // Backtest data
  const [candidates, setCandidates] = useState<BacktestCandidate[]>([]);
  const [selectedMatchId, setSelectedMatchId] = useState<number | null>(null);
  const [evaluationResult, setEvaluationResult] = useState<BacktestEvaluateResponse | null>(null);

  const canLoad = useMemo(
    () => !!format && !!team1 && !!team2 && !loading,
    [format, team1, team2, loading],
  );
  const canEvaluate = useMemo(
    () => !!format && !!team1 && !!team2 && selectedMatchId != null && !loading,
    [format, team1, team2, selectedMatchId, loading],
  );

  const resetOutputs = () => {
    setCandidates([]);
    setSelectedMatchId(null);
    setEvaluationResult(null);
    setStatusMessage('');
    setError(null);
  };

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
    try {
      setLoading(true);
      setError(null);
      setStatusMessage('Evaluating…');
      const resp = await api.backtestEvaluate(format, team1.trim(), team2.trim(), selectedMatchId);
      setEvaluationResult(resp);
      setStatusMessage('Evaluation complete.');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setStatusMessage('');
    } finally {
      setLoading(false);
    }
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
            {availableFormats.length === 0 && <MenuItem value={format}>{format}</MenuItem>}
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
          options={availableTeams.filter((t) => t !== team2)}
          value={team1}
          onChange={(_e, newValue) => {
            if (newValue) {
              setTeam1(newValue);
              resetOutputs();
            }
          }}
          filterOptions={(options, params) => {
            const filtered = filter(options, params);
            if (params.inputValue !== '' && params.inputValue.length < 3) {
              return [];
            }
            return filtered;
          }}
          renderInput={(params) => <TextField {...params} label="Team 1" />}
          noOptionsText="Type at least 3 characters"
        />

        <Autocomplete
          fullWidth
          size="small"
          disableClearable
          options={availableTeams.filter((t) => t !== team1)}
          value={team2}
          onChange={(_e, newValue) => {
            if (newValue) {
              setTeam2(newValue);
              resetOutputs();
            }
          }}
          filterOptions={(options, params) => {
            const filtered = filter(options, params);
            if (params.inputValue !== '' && params.inputValue.length < 3) {
              return [];
            }
            return filtered;
          }}
          renderInput={(params) => <TextField {...params} label="Team 2" />}
          noOptionsText="Type at least 3 characters"
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
        <Box sx={{ mt: 2 }}>
          <Button
            variant="contained"
            color="secondary"
            onClick={handleEvaluateSelectedMatch}
            disabled={!canEvaluate}
          >
            Evaluate Selected Match
          </Button>
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
