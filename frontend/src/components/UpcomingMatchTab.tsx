import React from 'react';
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
} from '@mui/material';
import type { SxProps } from '@mui/material';
import Autocomplete, { createFilterOptions } from '@mui/material/Autocomplete';
import TeamTable from './TeamTable';
import { useUpcomingMatch } from '../hooks/useUpcomingMatch';
import type { PredictScorecardSummary } from '../types';

const filter = createFilterOptions<string>();

function ScorecardSummaryDisplay({
  title,
  summaryData,
  team1,
  team2,
  titleColor,
  sx,
}: {
  title: string;
  summaryData: PredictScorecardSummary;
  team1: string;
  team2: string;
  titleColor?: string;
  sx?: SxProps;
}) {
  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 2, ...sx }}>
      <Typography
        variant="subtitle2"
        {...(titleColor ? { sx: { color: titleColor } } : { color: 'text.secondary' })}
        gutterBottom
      >
        {title}
      </Typography>
      <Stack direction="row" spacing={3} flexWrap="wrap">
        <Typography variant="body2">
          <strong>Innings 1 ({team1}):</strong> {summaryData.innings1_total.toFixed(0)} runs
        </Typography>
        <Typography variant="body2">
          <strong>Innings 2 ({team2}):</strong> {summaryData.innings2_total.toFixed(0)} runs
        </Typography>
        {summaryData.predicted_winner && (
          <Typography variant="body2">
            <strong>Predicted winner:</strong> {summaryData.predicted_winner}
          </Typography>
        )}
        {summaryData.team1_win_probability != null && (
          <Typography variant="body2">
            <strong>Win probability ({team1}):</strong>{' '}
            {(summaryData.team1_win_probability * 100).toFixed(1)}%
          </Typography>
        )}
      </Stack>
    </Paper>
  );
}

function teamFilterOptions(options: string[], params: Parameters<typeof filter>[1]): string[] {
  const filtered = filter(options, params);
  if (params.inputValue === '') return filtered;
  if (params.inputValue.length < 2) return [];
  return filtered;
}

const UpcomingMatchTab: React.FC = () => {
  const {
    format,
    setFormat,
    team1,
    setTeam1,
    team2,
    setTeam2,
    venue,
    setVenue,
    matchDate,
    setMatchDate,
    runSimulation,
    setRunSimulation,
    availableFormats,
    availableTeam1s,
    availableTeam2s,
    venueOptions,
    venueLoading,
    handleVenueInputChange,
    loading,
    error,
    result,
    minDate,
    maxDate,
    dateError,
    canPredict,
    handlePredict,
    maxFutureDays,
  } = useUpcomingMatch();

  return (
    <Box>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Upcoming match prediction
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Select format, teams, venue (optional), and a future match date. Date must be today or
        within {maxFutureDays} days to ensure accurate predictions.
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
          inputProps={{ min: minDate, max: maxDate }}
          error={!!dateError}
          helperText={dateError}
          fullWidth
        />

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
            <ScorecardSummaryDisplay
              title="Scorecard summary"
              summaryData={result.scorecard_summary}
              team1={team1}
              team2={team2}
              sx={{ bgcolor: 'grey.50' }}
            />
          )}
          {result.scorecard_summary_reconciled && (
            <ScorecardSummaryDisplay
              title="Reconciled scorecard summary (aligned with win model)"
              summaryData={result.scorecard_summary_reconciled}
              team1={team1}
              team2={team2}
              titleColor="primary.dark"
              sx={{ bgcolor: 'primary.50' }}
            />
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
                  <strong>Draw:</strong>
                  {(result.simulation.draw_probability * 100).toFixed(1)}%
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

export default UpcomingMatchTab;
