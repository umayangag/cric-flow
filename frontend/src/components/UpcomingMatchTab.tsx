import React, { useMemo, useState, useEffect, useRef } from 'react';
import {
  Box,
  Button,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Stack,
  Switch,
  Typography,
  Alert,
  CircularProgress,
  TextField,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from '@mui/material';
import Autocomplete, { createFilterOptions } from '@mui/material/Autocomplete';
import { api } from '../api';
import type { PredictTeamSelectionResponse, PredictTeamSelectedPlayer } from '../types';

/** Maximum days in the future allowed for match date (predictions degrade beyond this). */
const MAX_FUTURE_DAYS = 14;

const filter = createFilterOptions<string>();

function teamFilterOptions(options: string[], params: Parameters<typeof filter>[1]): string[] {
  const filtered = filter(options, params);
  if (params.inputValue === '') return filtered;
  if (params.inputValue.length < 2) return [];
  return filtered;
}

function formatDateForInput(d: Date): string {
  return d.toISOString().slice(0, 10);
}

const UpcomingMatchTab: React.FC = () => {
  const [format, setFormat] = useState<string>('');
  const [team1, setTeam1] = useState<string>('');
  const [team2, setTeam2] = useState<string>('');
  const [venue, setVenue] = useState<string>('');
  const [matchDate, setMatchDate] = useState<string>('');
  const [predictionModel, setPredictionModel] = useState<'format' | 'unified'>('format');
  const [runSimulation, setRunSimulation] = useState<boolean>(false);

  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [availableTeam1s, setAvailableTeam1s] = useState<string[]>([]);
  const [availableTeam2s, setAvailableTeam2s] = useState<string[]>([]);
  const [venueOptions, setVenueOptions] = useState<string[]>([]);
  const [venueLoading, setVenueLoading] = useState<boolean>(false);
  const venueSearchRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<PredictTeamSelectionResponse | null>(null);

  // Date constraints: today and today + MAX_FUTURE_DAYS
  const { minDate, maxDate } = useMemo(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const max = new Date(today);
    max.setDate(max.getDate() + MAX_FUTURE_DAYS);
    return {
      minDate: formatDateForInput(today),
      maxDate: formatDateForInput(max),
    };
  }, []);

  useEffect(() => {
    let active = true;
    api
      .getFormats()
      .then((f) => {
        if (active) setAvailableFormats(f);
      })
      .catch((e) => {
        if (active)
          setError(`Failed to load formats: ${e instanceof Error ? e.message : String(e)}`);
      });
    return () => {
      active = false;
    };
  }, []);

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
          setTeam1((prev) => (prev && !teams.includes(prev) ? '' : prev));
        }
      })
      .catch((e) => {
        if (active) setError(`Failed to load teams: ${e instanceof Error ? e.message : String(e)}`);
      });
    return () => {
      active = false;
    };
  }, [format]);

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
          setTeam2((prev) => (prev && !opps.includes(prev) ? '' : prev));
        }
      })
      .catch((e) => {
        if (active)
          setError(`Failed to load opponents: ${e instanceof Error ? e.message : String(e)}`);
      });
    return () => {
      active = false;
    };
  }, [format, team1]);

  const fetchVenueOptions = (query: string) => {
    const trimmed = query.trim();
    if (trimmed.length < 3) {
      setVenueOptions([]);
      return;
    }
    setVenueLoading(true);
    api
      .searchVenues(trimmed)
      .then((list) => setVenueOptions(list))
      .catch(() => setVenueOptions([]))
      .finally(() => setVenueLoading(false));
  };

  const handleVenueInputChange = (_: React.SyntheticEvent, value: string) => {
    setVenue(value);
    if (venueSearchRef.current) {
      clearTimeout(venueSearchRef.current);
      venueSearchRef.current = null;
    }
    if (value.trim().length < 3) {
      setVenueOptions([]);
      return;
    }
    venueSearchRef.current = setTimeout(() => fetchVenueOptions(value), 300);
  };

  useEffect(() => {
    return () => {
      if (venueSearchRef.current) clearTimeout(venueSearchRef.current);
    };
  }, []);

  const dateError = useMemo(() => {
    if (!matchDate) return 'Match date is required';
    // Parse YYYY-MM-DD as local date (input[type=date] gives calendar date; new Date(str) parses as UTC).
    const [y, m, d] = matchDate.split('-').map(Number);
    const selectedLocal = new Date(y, m - 1, d);
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const max = new Date(today);
    max.setDate(max.getDate() + MAX_FUTURE_DAYS);
    if (selectedLocal < today) return 'Date must be today or in the future';
    if (selectedLocal > max) return `Date must be within ${MAX_FUTURE_DAYS} days from today`;
    return null;
  }, [matchDate]);

  const canPredict = useMemo(
    () => !!format && !!team1 && !!team2 && !!matchDate && !dateError && !loading,
    [format, team1, team2, matchDate, dateError, loading],
  );

  const handlePredict = async () => {
    if (!canPredict) return;
    setError(null);
    setResult(null);
    setLoading(true);
    try {
      const res = await api.predictTeamSelection({
        format: format.trim(),
        team1: team1.trim(),
        team2: team2.trim(),
        venue: venue.trim() || undefined,
        match_date: matchDate,
        use_unified_model: predictionModel === 'unified',
        simulate: runSimulation,
      });
      setResult(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Upcoming match prediction
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Select format, teams, venue (optional), and a future match date. Date must be today or
        within {MAX_FUTURE_DAYS} days to ensure accurate predictions.
      </Typography>

      <Stack spacing={2} sx={{ mb: 3 }}>
        <FormControl size="small" sx={{ minWidth: 120 }}>
          <InputLabel>Format</InputLabel>
          <Select value={format} onChange={(e) => setFormat(e.target.value)} label="Format">
            <MenuItem value="">
              <em>Select format</em>
            </MenuItem>
            {availableFormats.map((f) => (
              <MenuItem key={f} value={f}>
                {f}
              </MenuItem>
            ))}
          </Select>
        </FormControl>

        <Autocomplete
          size="small"
          options={availableTeam1s}
          value={team1}
          onChange={(_, v) => setTeam1(v ?? '')}
          onInputChange={(_, v) => setTeam1(v)}
          filterOptions={teamFilterOptions}
          renderInput={(params) => (
            <TextField {...params} label="Team 1" placeholder="Select or type team" />
          )}
        />

        <Autocomplete
          size="small"
          options={availableTeam2s}
          value={team2}
          onChange={(_, v) => setTeam2(v ?? '')}
          onInputChange={(_, v) => setTeam2(v)}
          filterOptions={teamFilterOptions}
          renderInput={(params) => (
            <TextField {...params} label="Team 2" placeholder="Select or type team" />
          )}
        />

        <Autocomplete
          size="small"
          freeSolo
          options={venueOptions}
          value={venue}
          onInputChange={handleVenueInputChange}
          onChange={(_, v) => setVenue(typeof v === 'string' ? v : (v ?? ''))}
          filterOptions={(opts) => opts}
          loading={venueLoading}
          renderInput={(params) => (
            <TextField
              {...params}
              label="Venue (optional)"
              placeholder="Type 3+ characters to search venues"
            />
          )}
        />

        <TextField
          size="small"
          type="date"
          label="Match date"
          value={matchDate}
          onChange={(e) => setMatchDate(e.target.value)}
          InputLabelProps={{ shrink: true }}
          inputProps={{
            min: minDate,
            max: maxDate,
          }}
          error={!!dateError}
          helperText={dateError}
          fullWidth
        />

        <FormControl size="small" sx={{ minWidth: 260 }}>
          <InputLabel id="upcoming-model-label">Prediction model</InputLabel>
          <Select
            labelId="upcoming-model-label"
            value={predictionModel}
            onChange={(e) => setPredictionModel(e.target.value as 'format' | 'unified')}
            label="Prediction model"
          >
            <MenuItem value="format">Format-specific (model for selected format)</MenuItem>
            <MenuItem value="unified">Unified (all-formats / legacy model)</MenuItem>
          </Select>
        </FormControl>

        <FormControlLabel
          control={
            <Switch
              checked={runSimulation}
              onChange={(_, checked) => setRunSimulation(checked)}
              color="primary"
            />
          }
          label="Run Monte Carlo simulation (win prob & innings distribution over top XIs)"
        />

        <Button
          variant="contained"
          onClick={handlePredict}
          disabled={!canPredict}
          startIcon={loading ? <CircularProgress size={16} color="inherit" /> : null}
        >
          {loading ? 'Predicting…' : 'Predict best 11'}
        </Button>
      </Stack>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {result && (
        <Box sx={{ mt: 3 }}>
          {result.scorecard_summary && (
            <Paper variant="outlined" sx={{ p: 2, mb: 2, bgcolor: 'grey.50' }}>
              <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                Scorecard summary (winner from win model when available)
              </Typography>
              <Stack direction="row" spacing={3} flexWrap="wrap">
                <Typography variant="body2">
                  <strong>Innings 1 ({team1}):</strong>{' '}
                  {result.scorecard_summary.innings1_total.toFixed(0)} runs
                </Typography>
                <Typography variant="body2">
                  <strong>Innings 2 ({team2}):</strong>{' '}
                  {result.scorecard_summary.innings2_total.toFixed(0)} runs
                </Typography>
                {result.scorecard_summary.predicted_winner && (
                  <Typography variant="body2">
                    <strong>Predicted winner:</strong> {result.scorecard_summary.predicted_winner}
                  </Typography>
                )}
                {result.scorecard_summary.team1_win_probability != null && (
                    <Typography variant="body2">
                      <strong>Win probability ({team1}):</strong>{' '}
                      {(result.scorecard_summary.team1_win_probability * 100).toFixed(1)}%
                    </Typography>
                  )}
              </Stack>
            </Paper>
          )}
          {result.simulation && (
            <Paper variant="outlined" sx={{ p: 2, mb: 2, bgcolor: 'primary.50' }}>
              <Typography variant="subtitle2" color="primary.dark" gutterBottom>
                Monte Carlo simulation (over top XIs and sampled outcomes)
              </Typography>
              <Stack direction="row" spacing={3} flexWrap="wrap" sx={{ mb: 1 }}>
                <Typography variant="body2">
                  <strong>Win prob ({team1}):</strong>{' '}
                  {(result.simulation.win_probability_team1 * 100).toFixed(1)}%
                </Typography>
                <Typography variant="body2">
                  <strong>Win prob ({team2}):</strong>{' '}
                  {(result.simulation.win_probability_team2 * 100).toFixed(1)}%
                </Typography>
                <Typography variant="body2">
                  <strong>Draw:</strong>{(result.simulation.draw_probability * 100).toFixed(1)}%
                </Typography>
              </Stack>
              <Typography variant="caption" color="text.secondary" display="block" sx={{ mb: 0.5 }}>
                Innings 1 total: P10 = {result.simulation.innings1_total_p10.toFixed(0)} · P50 ={' '}
                {result.simulation.innings1_total_p50.toFixed(0)} · P90 ={' '}
                {result.simulation.innings1_total_p90.toFixed(0)}
              </Typography>
              <Typography variant="caption" color="text.secondary" display="block">
                Innings 2 total: P10 = {result.simulation.innings2_total_p10.toFixed(0)} · P50 ={' '}
                {result.simulation.innings2_total_p50.toFixed(0)} · P90 ={' '}
                {result.simulation.innings2_total_p90.toFixed(0)}
              </Typography>
              <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 0.5 }}>
                {result.simulation.num_matchups} matchups × {result.simulation.num_samples} samples
              </Typography>
            </Paper>
          )}
          <Typography variant="subtitle1" sx={{ mb: 1, fontWeight: 600 }}>
            Best 11 for each team
          </Typography>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
            <TeamTable teamName={team1} players={result.team1} />
            <TeamTable teamName={team2} players={result.team2} />
          </Stack>
        </Box>
      )}
    </Box>
  );
};

function TeamTable({
  teamName,
  players,
}: {
  teamName: string;
  players: PredictTeamSelectedPlayer[];
}) {
  return (
    <Paper variant="outlined" sx={{ flex: 1, overflow: 'hidden' }}>
      <Typography variant="subtitle2" sx={{ px: 2, py: 1, bgcolor: 'action.hover' }}>
        {teamName}
      </Typography>
      <TableContainer>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Player</TableCell>
              <TableCell align="right">Runs</TableCell>
              <TableCell align="right">Wkts</TableCell>
              <TableCell align="right">Econ</TableCell>
              <TableCell align="right">Catches</TableCell>
              <TableCell align="right">RO</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {players.map((p) => (
              <TableRow key={p.player_id}>
                <TableCell>{p.player_name}</TableCell>
                <TableCell align="right">{p.runs.toFixed(1)}</TableCell>
                <TableCell align="right">{p.wickets.toFixed(1)}</TableCell>
                <TableCell align="right">{p.economy.toFixed(2)}</TableCell>
                <TableCell align="right">{p.catches.toFixed(0)}</TableCell>
                <TableCell align="right">{p.run_outs.toFixed(0)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Paper>
  );
}

export default UpcomingMatchTab;
