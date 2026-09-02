import React from 'react';
import { Chip, Paper, Stack, Typography } from '@mui/material';
import { MetricInfo } from './common/MetricInfo';
import type { PredictInningsTotal, PredictScorecard, PredictWinProbability } from '../types';

type Props = {
  /** Absent for a format with no innings length: there is no total, and none is invented. */
  scorecard?: PredictScorecard;
  winProbability: PredictWinProbability;
  team1: string;
  team2: string;
};

const winProbabilitySources: Record<PredictWinProbability['source'], string> = {
  display: 'display model (monotone GBM over both elevens)',
  simulator: 'simulator (share of simulated matches won)',
};

const InningsLine: React.FC<{ label: string; innings: PredictInningsTotal }> = ({
  label,
  innings,
}) => (
  <Typography variant="body2">
    <strong>{label}:</strong> {innings.total.toFixed(0)} runs ({innings.p10.toFixed(0)}–
    {innings.p90.toFixed(0)}), extras {innings.extras.toFixed(0)}
    <MetricInfo
      metricKey="range_10_90"
      label={`${label} 10–90 range`}
      value={`${innings.p10.toFixed(0)}–${innings.p90.toFixed(0)}`}
    />
  </Typography>
);

/**
 * The predicted match: the innings totals with the range the draws produced, and the
 * headline win probability with the model it came from.
 *
 * The totals and the per-player lines come from one set of draws, so they sum: nothing here
 * is rescaled toward the win probability, which is what the two estimates used to be pulled
 * together into. Where the format has no innings length there is no total at all, and the
 * card says so rather than summing eleven medians and calling it an innings.
 */
const MatchScorecard: React.FC<Props> = ({ scorecard, winProbability, team1, team2 }) => {
  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
      <Typography variant="subtitle2" color="text.secondary" gutterBottom>
        Predicted match
      </Typography>
      <Stack direction="row" spacing={3} flexWrap="wrap" sx={{ mb: 1 }}>
        <Typography variant="body2">
          <strong>Win probability ({team1}):</strong> {(winProbability.team1 * 100).toFixed(1)}%
          <MetricInfo
            metricKey="win_probability"
            label="Win probability"
            value={`${(winProbability.team1 * 100).toFixed(1)}%`}
          />
        </Typography>
        <Typography variant="body2">
          <strong>Predicted winner:</strong> {winProbability.predicted_winner}
        </Typography>
        {winProbability.simulated != null && (
          <Typography variant="body2" color="text.secondary">
            simulated: {(winProbability.simulated * 100).toFixed(1)}%
          </Typography>
        )}
      </Stack>
      <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ mb: 1 }}>
        <Chip
          size="small"
          variant="outlined"
          label={`source: ${winProbabilitySources[winProbability.source]}`}
        />
        {scorecard && (
          <>
            <Chip
              size="small"
              variant="outlined"
              label={`${scorecard.samples.toLocaleString()} draws`}
            />
            {scorecard.toss_marginalised && (
              <Chip size="small" variant="outlined" label="toss unknown: both orders averaged" />
            )}
          </>
        )}
      </Stack>
      {scorecard ? (
        <Stack spacing={0.5}>
          <InningsLine label={`Innings 1 (${team1})`} innings={scorecard.innings1} />
          <InningsLine label={`Innings 2 (${team2})`} innings={scorecard.innings2} />
          <Typography variant="caption" color="text.secondary">
            Totals and the per-player lines come from the same draws, so the lines and extras sum to
            the total shown. Nothing is rescaled toward the win probability.
          </Typography>
        </Stack>
      ) : (
        <Typography variant="caption" color="text.secondary">
          This format has no fixed innings length, so there is no simulated total. The per-player
          numbers are the performance model’s own medians and 10–90 intervals.
        </Typography>
      )}
    </Paper>
  );
};

export default MatchScorecard;
