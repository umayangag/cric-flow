import React from 'react';
import { Chip, Stack, Tooltip, Typography } from '@mui/material';
import type { PredictConstraintReport, PredictConstraintStatus } from '../types';

/**
 * What was asked of a hand-built eleven, and what it actually holds (P1-2).
 *
 * Every chip is state the response reported, not a rule this component applies: the bowler
 * count and the keeper flag are ml-service's, measured on the eleven that was scored, so
 * "4 of 5 bowlers" counts what the optimiser would have counted. A broken constraint is
 * shown broken and the eleven beside it is the one the numbers describe — nothing was
 * repaired to make a chip green.
 */

export type ConstraintChipsProps = {
  report: PredictConstraintReport;
  side: 1 | 2;
  teamName: string;
};

function bowlerChip(status: PredictConstraintStatus, minBowlers: number) {
  const met = status.bowlers >= minBowlers;
  return {
    key: 'bowlers',
    label: `Bowlers ${status.bowlers} of ${minBowlers}`,
    met,
    tooltip: met
      ? 'This eleven holds at least the minimum bowling options asked for.'
      : 'This eleven is short of bowling options. It was scored as you built it, not repaired.',
  };
}

function keeperChip(status: PredictConstraintStatus, requireKeeper: boolean) {
  const met = status.has_keeper || !requireKeeper;
  return {
    key: 'keeper',
    label: status.has_keeper ? 'Keeper in the side' : 'No keeper',
    met,
    tooltip: requireKeeper
      ? 'A wicketkeeper was required of this eleven.'
      : 'No wicketkeeper was required, so this is reported and not judged.',
  };
}

function sizeChip(status: PredictConstraintStatus, teamSize: number) {
  return {
    key: 'size',
    label: `${status.size} of ${teamSize} players`,
    met: status.size === teamSize,
    tooltip: 'An eleven is scored as an eleven: every model here aggregates a whole side.',
  };
}

const ConstraintChips: React.FC<ConstraintChipsProps> = ({ report, side, teamName }) => {
  const status = side === 1 ? report.team1 : report.team2;
  const chips = [
    sizeChip(status, report.team_size),
    bowlerChip(status, report.min_bowlers),
    keeperChip(status, report.require_keeper),
  ];
  const missing = status.missing_must_include ?? [];

  return (
    <Stack
      direction="row"
      spacing={1}
      useFlexGap
      flexWrap="wrap"
      sx={{ mb: 1 }}
      aria-label={`Constraints for ${teamName}`}
    >
      {chips.map((chip) => (
        <Tooltip key={chip.key} title={chip.tooltip}>
          <Chip
            size="small"
            variant={chip.met ? 'outlined' : 'filled'}
            color={chip.met ? 'default' : 'warning'}
            label={chip.met ? chip.label : `${chip.label} — broken`}
          />
        </Tooltip>
      ))}
      {missing.map((player) => (
        <Tooltip
          key={player.player_id}
          title="You asked for this player to be included, and this eleven does not hold him."
        >
          <Chip
            size="small"
            color="warning"
            label={`Must include ${player.player_name} — broken`}
          />
        </Tooltip>
      ))}
      {!status.met && (
        <Typography variant="caption" color="text.secondary" sx={{ alignSelf: 'center' }}>
          Scored as you built it; nothing was substituted.
        </Typography>
      )}
    </Stack>
  );
};

export default ConstraintChips;
