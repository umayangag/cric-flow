import React from 'react';
import { Box, Paper, Stack, Typography } from '@mui/material';
import { MetricLabel } from './common/MetricInfo';
import { changeDirection, formatChange, formatProbabilityChange } from '../lib/playDelta';
import type { PlayDelta } from '../lib/playDelta';

/**
 * What the last change did (P1-2): the headline probability, and each side's simulated
 * innings with its 10-90 range.
 *
 * Every number here is one response's value minus another's, and both responses are on
 * screen — the current answer above, the previous one only as the difference. The change
 * is never a forecast of its own: it is the arithmetic in {@link playDelta}, nothing more.
 * Each label opens the explainer of the number it is a change in (P1-4).
 */

export type PlayDeltaSummaryProps = {
  delta: PlayDelta | null;
  team1: string;
  team2: string;
};

const DIRECTION_COLOR = {
  up: 'success.main',
  down: 'error.main',
  none: 'text.secondary',
} as const;

const ChangeValue: React.FC<{
  metricKey: string;
  label: string;
  change: number;
  text: string;
}> = ({ metricKey, label, change, text }) => (
  <Box>
    <Typography variant="caption" color="text.secondary" component="div">
      <MetricLabel metricKey={metricKey} label={label} />
    </Typography>
    <Typography variant="body2" sx={{ color: DIRECTION_COLOR[changeDirection(change)] }}>
      {text}
    </Typography>
  </Box>
);

const PlayDeltaSummary: React.FC<PlayDeltaSummaryProps> = ({ delta, team1, team2 }) => {
  if (!delta) return null;
  const innings: { name: string; total: number; p10: number; p90: number }[] = [];
  if (delta.team1Innings) {
    innings.push({
      name: team1,
      total: delta.team1Innings.total.change,
      p10: delta.team1Innings.p10.change,
      p90: delta.team1Innings.p90.change,
    });
  }
  if (delta.team2Innings) {
    innings.push({
      name: team2,
      total: delta.team2Innings.total.change,
      p10: delta.team2Innings.p10.change,
      p90: delta.team2Innings.p90.change,
    });
  }

  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 2 }} aria-label="Change from the previous eleven">
      <Typography variant="subtitle2" gutterBottom>
        Change from the previous eleven
      </Typography>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={3} useFlexGap flexWrap="wrap">
        <ChangeValue
          metricKey="win_probability"
          label={`P(${team1} wins)`}
          change={delta.winProbability.change}
          text={formatProbabilityChange(delta.winProbability.change)}
        />
        {innings.map((entry) => (
          <ChangeValue
            key={entry.name}
            metricKey="innings_total"
            label={`${entry.name} innings (10-90)`}
            change={entry.total}
            text={`${formatChange(entry.total, 1)} runs (${formatChange(entry.p10, 1)} / ${formatChange(entry.p90, 1)})`}
          />
        ))}
      </Stack>
      <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
        Each figure is this answer minus the previous one. Both were served from the same rating
        run; a change measured across a reload is not shown at all.
      </Typography>
    </Paper>
  );
};

export default PlayDeltaSummary;
