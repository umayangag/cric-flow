import React from 'react';
import {
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  createFilterOptions,
  Divider,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import type { LabConstraints, TeamLabState, TossChoice } from '../hooks/useTeamLab';
import type { TeamSideOption } from '../types';

/**
 * Everything the Lab is asked before it answers: the fixture, each side's candidate pool,
 * the toss and the constraints.
 *
 * It is presentational — every value and every setter comes from {@link useTeamLab}, which
 * is also the one place the request is built, so there is one surface on
 * `POST /api/predict/team-selection` and nothing here can drift from it.
 */

// Sides are matched on their display name -- "India (men)" -- so typing "women" narrows the
// list to the women's sides, which is the distinction the picker exists to make (D-10).
const filter = createFilterOptions<TeamSideOption>({ stringify: (side) => side.display_name });

function teamFilterOptions(
  options: TeamSideOption[],
  params: Parameters<typeof filter>[1],
): TeamSideOption[] {
  const filtered = filter(options, params);
  if (params.inputValue === '') return filtered;
  if (params.inputValue.length < 2) return [];
  return filtered;
}

/** Two sides are the same option when they are the same club, never when they share a name. */
function isSameSide(option: TeamSideOption, value: TeamSideOption): boolean {
  return option.club_id === value.club_id;
}

/** The three states of the toss, in the order a user meets them (P1-1). */
const TOSS_OPTIONS: { value: TossChoice; label: string }[] = [
  { value: 'unknown', label: 'Toss unknown' },
  { value: 'team1_bats_first', label: 'Team 1 bats first' },
  { value: 'team2_bats_first', label: 'Team 2 bats first' },
];

export type TeamLabInputsProps = {
  lab: TeamLabState;
  /** Open one side's candidate list; manual picking is optional and starts closed (D-12). */
  onOpenPool: (side: 1 | 2) => void;
};

const TeamLabInputs: React.FC<TeamLabInputsProps> = ({ lab, onOpenPool }) => {
  const sides: { side: 1 | 2; team: TeamSideOption | null; players: number[] | null }[] = [
    { side: 1, team: lab.team1, players: lab.team1Pool.players },
    { side: 2, team: lab.team2, players: lab.team2Pool.players },
  ];

  // One field changes, the rest are kept. The new value is read from the event here and
  // not inside the updater, which runs later and would read the input again by then.
  const changeConstraint = <K extends keyof LabConstraints>(
    field: K,
    value: LabConstraints[K],
  ): void => {
    lab.setConstraints((current) => ({ ...current, [field]: value }));
  };

  return (
    <Stack spacing={2} sx={{ mb: 3 }}>
      <FormControl size="small" sx={{ minWidth: 120 }}>
        <InputLabel id="team-lab-format-label">Format</InputLabel>
        <Select
          labelId="team-lab-format-label"
          value={lab.format}
          onChange={(e) => lab.setFormat(e.target.value)}
          label="Format"
        >
          <MenuItem value="">
            <em>Select format</em>
          </MenuItem>
          {lab.availableFormats.map((f) => (
            <MenuItem key={f} value={f}>
              {f}
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <Autocomplete
        size="small"
        options={lab.availableTeam1s}
        value={lab.team1}
        onChange={(_, v) => lab.setTeam1(v)}
        getOptionLabel={(side) => side.display_name}
        isOptionEqualToValue={isSameSide}
        filterOptions={teamFilterOptions}
        renderInput={(params) => (
          <TextField {...params} label="Team 1" placeholder="Select or type team" />
        )}
      />

      <Autocomplete
        size="small"
        options={lab.availableTeam2s}
        value={lab.team2}
        onChange={(_, v) => lab.setTeam2(v)}
        getOptionLabel={(side) => side.display_name}
        isOptionEqualToValue={isSameSide}
        filterOptions={teamFilterOptions}
        renderInput={(params) => (
          <TextField {...params} label="Team 2" placeholder="Select or type team" />
        )}
      />

      <Autocomplete
        size="small"
        freeSolo
        options={lab.venueOptions}
        value={lab.venue}
        onInputChange={lab.handleVenueInputChange}
        onChange={(_, v) => lab.setVenue(typeof v === 'string' ? v : (v ?? ''))}
        filterOptions={(opts) => opts}
        loading={lab.venueLoading}
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
        value={lab.matchDate}
        onChange={(e) => lab.setMatchDate(e.target.value)}
        InputLabelProps={{ shrink: true }}
        inputProps={{ min: lab.minDate, max: lab.maxDate }}
        error={!!lab.dateError}
        helperText={lab.dateError}
        fullWidth
      />

      {/*
        The candidate pool is bounded by recency now, and this is where a user widens it
        or picks by hand. Touching neither control is the default and the unchanged flow
        (D-12).
      */}
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
        {sides.map((entry) => (
          <Button
            key={entry.side}
            size="small"
            variant="outlined"
            disabled={!lab.format || !entry.team}
            onClick={() => onOpenPool(entry.side)}
          >
            {entry.players?.length
              ? `Team ${entry.side} pool: ${entry.players.length} chosen`
              : `Choose team ${entry.side} candidates`}
          </Button>
        ))}
      </Stack>

      <Divider />

      {/*
        The toss. "Unknown" is not a missing answer: it is the simulator drawing half the
        matches each way, which the response reports as `toss_marginalised` and the
        scorecard names. The three states are three different requests (P1-1).
      */}
      <Box>
        <Typography variant="subtitle2" gutterBottom>
          Toss
        </Typography>
        <ToggleButtonGroup
          size="small"
          exclusive
          value={lab.toss}
          onChange={(_, value: TossChoice | null) => value && lab.setToss(value)}
          aria-label="Toss"
        >
          {TOSS_OPTIONS.map((option) => (
            <ToggleButton key={option.value} value={option.value}>
              {option.label}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
          Unknown draws both batting orders and averages them, which is what the simulator has
          always done. The answer says which toss it assumed.
        </Typography>
      </Box>

      <Divider />

      <Box>
        <Typography variant="subtitle2" gutterBottom>
          Constraints
        </Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems="flex-start">
          <TextField
            size="small"
            type="number"
            label="Minimum bowlers"
            value={lab.constraints.minBowlers}
            onChange={(e) => changeConstraint('minBowlers', e.target.value)}
            inputProps={{ min: 1 }}
            helperText="Empty uses the configured default"
          />
          <FormControlLabel
            control={
              <Switch
                size="small"
                checked={lab.constraints.requireKeeper}
                onChange={(e) => changeConstraint('requireKeeper', e.target.checked)}
              />
            }
            label="Require a wicketkeeper"
          />
        </Stack>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} sx={{ mt: 2 }}>
          <TextField
            size="small"
            fullWidth
            label="Team 1 must-include player ids"
            placeholder="e.g. 4021, 5518"
            value={lab.constraints.extraTeam1}
            onChange={(e) => changeConstraint('extraTeam1', e.target.value)}
          />
          <TextField
            size="small"
            fullWidth
            label="Team 2 must-include player ids"
            placeholder="e.g. 4021, 5518"
            value={lab.constraints.extraTeam2}
            onChange={(e) => changeConstraint('extraTeam2', e.target.value)}
          />
        </Stack>
        <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
          Must-include ids join the candidate pool whatever the recency window or the retirement
          ledger says — a new signing, or a player you know is available. They are candidates the
          selection must consider, not players it must pick. Ids are shown beside each name in the
          candidate list.
        </Typography>
        {lab.constraintsError && (
          <Typography variant="caption" color="error" component="div" sx={{ mt: 0.5 }}>
            {lab.constraintsError}
          </Typography>
        )}
      </Box>

      <Button
        variant="contained"
        onClick={lab.handlePredict}
        disabled={!lab.canPredict}
        startIcon={lab.loading ? <CircularProgress size={16} color="inherit" /> : null}
      >
        {lab.loading ? 'Optimising…' : 'Optimise'}
      </Button>
    </Stack>
  );
};

export default TeamLabInputs;
