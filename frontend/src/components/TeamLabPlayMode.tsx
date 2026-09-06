import React from 'react';
import {
  Alert,
  AlertTitle,
  Box,
  Button,
  Chip,
  CircularProgress,
  IconButton,
  List,
  ListItem,
  ListItemText,
  Paper,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material';
import ConstraintChips from './ConstraintChips';
import PlayDeltaSummary from './PlayDeltaSummary';
import type { PlayModeState, PlayPlayer, PlaySide } from '../hooks/usePlayMode';
import type { PredictTeamSelectionResponse, TeamSideOption } from '../types';

/**
 * Play mode: add, remove or swap a player on either side, and see the fixture re-scored
 * (P1-2).
 *
 * The board is the eleven the user is building; the numbers under it are the answer to the
 * eleven that was last scored. While the two agree — which is every state except an
 * unfinished edit — the surface says nothing about it; while they do not, it says so
 * plainly rather than letting the numbers stand for an eleven nobody sent.
 *
 * Nothing here computes: the constraint chips are the response's, the delta is the
 * subtraction in `playDelta`, and the probability, totals and scorecard are Optimise's own
 * — the same models, reached by the same call with the elevens named.
 */

export type TeamLabPlayModeProps = {
  play: PlayModeState;
  result: PredictTeamSelectionResponse;
  team1: TeamSideOption | null;
  team2: TeamSideOption | null;
  loading: boolean;
  /** Search for the eleven again, leaving Play mode's edits behind. */
  onOptimise: () => void;
  /** Open the picker for an addition (no player replaced) or a swap. */
  onPick: (side: PlaySide, replacing: PlayPlayer | null) => void;
};

const SidePanel: React.FC<{
  side: PlaySide;
  teamName: string;
  players: PlayPlayer[];
  teamSize: number;
  result: PredictTeamSelectionResponse;
  disabled: boolean;
  onPick: (side: PlaySide, replacing: PlayPlayer | null) => void;
  onRemove: (side: PlaySide, playerId: number) => void;
}> = ({ side, teamName, players, teamSize, result, disabled, onPick, onRemove }) => (
  <Paper variant="outlined" sx={{ p: 2, flex: 1 }}>
    <Typography variant="subtitle2" gutterBottom>
      {teamName}
    </Typography>
    {result.constraints && (
      <ConstraintChips report={result.constraints} side={side} teamName={teamName} />
    )}
    <List dense disablePadding aria-label={`${teamName} eleven`}>
      {players.map((player) => (
        <ListItem
          key={player.player_id}
          disableGutters
          secondaryAction={
            <Stack direction="row" spacing={1}>
              <Button size="small" disabled={disabled} onClick={() => onPick(side, player)}>
                Swap
              </Button>
              <Tooltip title="Remove him; the eleven is not scored again until it is a full eleven">
                <span>
                  <IconButton
                    size="small"
                    aria-label={`Remove ${player.player_name}`}
                    disabled={disabled}
                    onClick={() => onRemove(side, player.player_id)}
                  >
                    ×
                  </IconButton>
                </span>
              </Tooltip>
            </Stack>
          }
        >
          <ListItemText primary={player.player_name} />
        </ListItem>
      ))}
    </List>
    <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 1 }}>
      <Button
        size="small"
        variant="outlined"
        disabled={disabled || players.length >= teamSize}
        onClick={() => onPick(side, null)}
      >
        Add a player
      </Button>
      <Chip
        size="small"
        variant="outlined"
        color={players.length === teamSize ? 'default' : 'warning'}
        label={`${players.length} of ${teamSize}`}
      />
    </Stack>
  </Paper>
);

const TeamLabPlayMode: React.FC<TeamLabPlayModeProps> = ({
  play,
  result,
  team1,
  team2,
  loading,
  onOptimise,
  onPick,
}) => {
  const team1Name = team1?.display_name ?? result.team1_side.display_name;
  const team2Name = team2?.display_name ?? result.team2_side.display_name;

  return (
    <Box sx={{ mt: 3 }}>
      <Stack direction="row" alignItems="center" spacing={2} sx={{ mb: 1 }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
          Play mode
        </Typography>
        {loading && <CircularProgress size={16} />}
        <Box sx={{ flex: 1 }} />
        <Button size="small" variant="outlined" disabled={loading} onClick={onOptimise}>
          Optimise again
        </Button>
      </Stack>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Add, remove or swap a player on either side and the fixture is scored again — the eleven you
        built, not one chosen for you. Optimise returns to the eleven this format&apos;s policy
        picks, which is the win-model search in T20I and ODI and the rating order in T20 and TEST.
      </Typography>

      <PlayDeltaSummary delta={play.delta} team1={team1Name} team2={team2Name} />

      {!play.complete && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          <AlertTitle>This eleven is not scored yet</AlertTitle>
          An eleven is scored as a whole side, so the numbers below are still the last eleven that
          was scored. Add a player to score the one on this board.
        </Alert>
      )}

      <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
        <SidePanel
          side={1}
          teamName={team1Name}
          players={play.team1}
          teamSize={play.teamSize}
          result={result}
          disabled={loading}
          onPick={onPick}
          onRemove={play.removePlayer}
        />
        <SidePanel
          side={2}
          teamName={team2Name}
          players={play.team2}
          teamSize={play.teamSize}
          result={result}
          disabled={loading}
          onPick={onPick}
          onRemove={play.removePlayer}
        />
      </Stack>
    </Box>
  );
};

export default TeamLabPlayMode;
