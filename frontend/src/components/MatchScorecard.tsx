import React from 'react';
import { Chip, Paper, Stack, Typography } from '@mui/material';
import { MetricInfo } from './common/MetricInfo';
import type {
  PredictInningsTotal,
  PredictScorecard,
  PredictTossSummary,
  PredictWinProbability,
} from '../types';

type Props = {
  /** Absent for a format with no innings length: there is no total, and none is invented. */
  scorecard?: PredictScorecard;
  winProbability: PredictWinProbability;
  /** Which batting order the numbers assume, straight off the wire (P1-1). */
  toss: PredictTossSummary;
  team1: string;
  team2: string;
};

/**
 * What the card says the toss was.
 *
 * The label is read off the response, not off the control the user last touched: a known
 * toss has to read as known, and a request that could not be honoured has to read as one
 * that was not (§8.7).
 */
function tossLabel(toss: PredictTossSummary, team1: string, team2: string): string {
  if (toss.team1_bats_first === null) return 'toss unknown: both batting orders averaged';
  return `toss: ${toss.team1_bats_first ? team1 : team2} bats first`;
}

/**
 * Name one side's innings.
 *
 * By the side, never by a batting position: the response carries team1's innings and
 * team2's innings whichever bats first, so "innings 1" beside "team 2 bats first" would be
 * a label contradicting the numbers under it. Where the toss is known the position is said
 * as well, because then it is known.
 */
function inningsLabel(team: string, toss: PredictTossSummary, isTeam1: boolean): string {
  if (toss.team1_bats_first === null) return `${team} innings`;
  const first = toss.team1_bats_first === isTeam1;
  return `${team} (batting ${first ? 'first' : 'second'})`;
}

/**
 * The two innings in the order they are played, where the toss says what that order is.
 *
 * Only the display order moves; both totals are the ones the response carried for their
 * own side, and neither is recomputed.
 */
function inningsInBattingOrder(
  scorecard: PredictScorecard,
  toss: PredictTossSummary,
  team1: string,
  team2: string,
): { label: string; total: PredictInningsTotal }[] {
  const lines = [
    { label: inningsLabel(team1, toss, true), total: scorecard.team1_innings },
    { label: inningsLabel(team2, toss, false), total: scorecard.team2_innings },
  ];
  return toss.team1_bats_first === false ? [lines[1], lines[0]] : lines;
}

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
const MatchScorecard: React.FC<Props> = ({ scorecard, winProbability, toss, team1, team2 }) => {
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
        <Chip size="small" variant="outlined" label={tossLabel(toss, team1, team2)} />
        {scorecard && (
          <Chip
            size="small"
            variant="outlined"
            label={`${scorecard.samples.toLocaleString()} draws`}
          />
        )}
      </Stack>
      {!toss.honoured && toss.note && (
        <Typography variant="caption" color="warning.main" component="div" sx={{ mb: 1 }}>
          {toss.note}
        </Typography>
      )}
      {scorecard ? (
        <Stack spacing={0.5}>
          {inningsInBattingOrder(scorecard, toss, team1, team2).map((innings) => (
            <InningsLine key={innings.label} label={innings.label} innings={innings.total} />
          ))}
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
