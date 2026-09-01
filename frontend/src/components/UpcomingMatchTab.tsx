import React from 'react';
import {
  Alert,
  AlertTitle,
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  createFilterOptions,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import TeamTable from './TeamTable';
import MatchScorecard from './MatchScorecard';
import ErrorNotice from './common/ErrorNotice';
import PredictionReadiness from './PredictionReadiness';
import { useUpcomingMatch } from '../hooks/useUpcomingMatch';

const filter = createFilterOptions<string>();

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
    opsStatus,
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

        <Button
          variant="contained"
          onClick={handlePredict}
          disabled={!canPredict}
          startIcon={loading ? <CircularProgress size={16} color="inherit" /> : null}
        >
          {loading ? 'Predicting…' : 'Predict best 11'}
        </Button>
      </Stack>

      <PredictionReadiness status={opsStatus} />

      <Box sx={{ mb: 2 }}>
        <ErrorNotice error={error} title="Prediction failed" />
      </Box>

      {result && (
        <Box sx={{ mt: 3 }}>
          <MatchScorecard
            scorecard={result.scorecard}
            winProbability={result.win_probability}
            team1={team1}
            team2={team2}
          />
          {!result.selection.optimised && (
            <Alert severity="info" sx={{ mb: 2 }}>
              <AlertTitle>Not optimised</AlertTitle>
              {result.selection.note ??
                'This format has no win objective that ranks, so the eleven is picked by as-of rating under the same constraints.'}
            </Alert>
          )}
          <Typography variant="subtitle1" sx={{ mb: 1, fontWeight: 600 }}>
            {result.selection.optimised
              ? 'Best 11 for each team'
              : 'Rating-ordered 11 for each team'}
          </Typography>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
            <TeamTable
              teamName={team1}
              players={result.team1}
              optimised={result.selection.optimised}
            />
            <TeamTable
              teamName={team2}
              players={result.team2}
              optimised={result.selection.optimised}
            />
          </Stack>
        </Box>
      )}
    </Box>
  );
};

export default UpcomingMatchTab;
