import React from 'react';
import { Chip, Paper, Stack, Typography } from '@mui/material';
import { MetricInfo, MetricLabel } from './common/MetricInfo';
import RatingsAsOf from './RatingsAsOf';
import PredictionRecordNote from './PredictionRecordNote';
import type {
  ForecastSource,
  PredictCompetitionLevelSummary,
  PredictForecastSummary,
  PredictInningsTotal,
  PredictRecordBlock,
  PredictScorecard,
  PredictSelectionSummary,
  PredictServedRatings,
  PredictTossSummary,
  PredictWinProbability,
  WinProbabilitySource,
} from '../types';

type Props = {
  /** Absent for a format with no innings length: there is no total, and none is invented. */
  scorecard?: PredictScorecard;
  winProbability: PredictWinProbability;
  /** Which model produced the per-player numbers, and why where it is not the simulator. */
  forecast: PredictForecastSummary;
  /**
   * How the elevens were chosen, so the probability can say what it is a read of: a
   * searched eleven, a rating-ordered one, or one the caller built (P1-4).
   */
  selection: PredictSelectionSummary;
  /** Which batting order the numbers assume, straight off the wire (P1-1). */
  toss: PredictTossSummary;
  /** Which competition level the displayed probability read, and how it was known (§8.7). */
  competitionLevel: PredictCompetitionLevelSummary;
  /**
   * Which rating state every number here was computed from, off the same payload (P1-5).
   * Never read from a status poll: that describes whatever is loaded now, which may be a
   * different run than the one that answered.
   */
  served: PredictServedRatings;
  /**
   * Whether this answer went on the prediction record, off the same payload (P2-3). It is
   * shown beside the served date because the two answer neighbouring questions: which
   * ratings produced the number, and whether the number will still be here to be scored.
   */
  record: PredictRecordBlock;
  team1: string;
  team2: string;
};

/**
 * What the card says the toss was, and which reading the numbers under it therefore are.
 *
 * The label is read off the response's `reading`, not off the control the user last
 * touched and not off the echoed toss: the two readings are different quantities, and the
 * chip is where a reader learns which of them is on screen (§8.7).
 */
function tossLabel(toss: PredictTossSummary, team1: string, team2: string): string {
  if (toss.reading === 'marginalised') return 'toss unknown: both batting orders averaged';
  return `toss: ${toss.team1_bats_first ? team1 : team2} bats first`;
}

/**
 * What the card says the competition level was, off the response's `reading` (§8.7): a
 * level read from both sides' history, or an average over both levels where no single
 * level could be read — a substitution the card names rather than hides.
 */
export function competitionLevelLabel(level: PredictCompetitionLevelSummary): string {
  if (level.reading === 'marginalised') return 'level unknown: both competition levels averaged';
  return `${level.level} fixture, from both sides' history`;
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
  if (toss.reading === 'marginalised') return `${team} innings`;
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

/**
 * Each source by the name the Lab shows it under, and the glossary key its explainer opens
 * from. The vocabulary is the contract's (H-24); the key is `<field>_<value>`, which is how
 * `ml/xi/glossary.py` names the entry, so a new source without an explainer is a label with
 * no popover — visible, not silent.
 */
const WIN_PROBABILITY_SOURCE_LABELS: Record<WinProbabilitySource, string> = {
  display: 'display model',
  simulator: 'simulator win share',
};
const FORECAST_SOURCE_LABELS: Record<ForecastSource, string> = {
  simulator: 'simulated match',
  performance_quantiles: 'performance quantiles',
};

export function winProbabilitySourceKey(source: WinProbabilitySource): string {
  return `win_probability_source_${source}`;
}

export function forecastSourceKey(source: ForecastSource): string {
  return `forecast_source_${source}`;
}

/**
 * What the headline probability is a read of, where no search produced it (P1-4).
 *
 * On a rating-ordered eleven (T20, TEST) and on one the caller built, the number is the
 * named model's opinion of that eleven and nothing more — not the result of a search that
 * maximised it. The notice at the XI says why; this says it beside the number.
 */
function readOfSentence(selection: PredictSelectionSummary, sourceLabel: string): string | null {
  if (selection.optimised) return null;
  const eleven =
    selection.objective === 'fixed' ? 'the eleven you built' : 'this rating-ordered eleven';
  return `This is the ${sourceLabel}'s read of ${eleven}, not the result of a search.`;
}

const InningsLine: React.FC<{ label: string; innings: PredictInningsTotal }> = ({
  label,
  innings,
}) => (
  <Typography variant="body2" component="div">
    <strong>
      <MetricLabel metricKey="innings_total" label={`${label}:`} />
    </strong>{' '}
    {innings.total.toFixed(0)} runs ({innings.p10.toFixed(0)}–{innings.p90.toFixed(0)}), extras{' '}
    {innings.extras.toFixed(0)}
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
 * card says so in the words the response gave, rather than summing eleven medians and
 * calling it an innings.
 */
const MatchScorecard: React.FC<Props> = ({
  scorecard,
  winProbability,
  forecast,
  selection,
  toss,
  competitionLevel,
  served,
  record,
  team1,
  team2,
}) => {
  const winSourceLabel = WIN_PROBABILITY_SOURCE_LABELS[winProbability.source];
  const readOf = readOfSentence(selection, winSourceLabel);
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
            <MetricInfo
              metricKey={winProbabilitySourceKey('simulator')}
              label="Simulated win share"
              value={`${(winProbability.simulated * 100).toFixed(1)}%`}
            />
          </Typography>
        )}
        <RatingsAsOf served={served} />
        <PredictionRecordNote record={record} />
      </Stack>
      <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap sx={{ mb: 1 }}>
        <Typography variant="body2" color="text.secondary" component="div">
          <MetricLabel
            metricKey={winProbabilitySourceKey(winProbability.source)}
            label={`source: ${winSourceLabel}`}
          />
        </Typography>
        <Typography variant="body2" color="text.secondary" component="div">
          <MetricLabel
            metricKey={forecastSourceKey(forecast.source)}
            label={`per-player numbers: ${FORECAST_SOURCE_LABELS[forecast.source]}`}
          />
        </Typography>
        <Chip size="small" variant="outlined" label={tossLabel(toss, team1, team2)} />
        <Chip
          size="small"
          variant="outlined"
          label={competitionLevelLabel(competitionLevel)}
          data-testid="competition-level-reading"
        />
        {scorecard && (
          <Chip
            size="small"
            variant="outlined"
            label={`${scorecard.samples.toLocaleString()} draws`}
          />
        )}
      </Stack>
      {readOf && (
        <Typography
          variant="caption"
          color="text.secondary"
          component="div"
          sx={{ mb: 1 }}
          data-testid="win-probability-read-of"
        >
          {readOf}
        </Typography>
      )}
      {toss.note && (
        <Typography variant="caption" color="text.secondary" component="div" sx={{ mb: 1 }}>
          {toss.note}
        </Typography>
      )}
      {competitionLevel.note && (
        <Typography
          variant="caption"
          color="text.secondary"
          component="div"
          sx={{ mb: 1 }}
          data-testid="competition-level-note"
        >
          {competitionLevel.note}
        </Typography>
      )}
      {scorecard ? (
        <Stack spacing={0.5}>
          {inningsInBattingOrder(scorecard, toss, team1, team2).map((innings) => (
            <InningsLine key={innings.label} label={innings.label} innings={innings.total} />
          ))}
          <Typography variant="caption" color="text.secondary">
            Totals and the per-player lines come from the same draws, so the lines and extras sum to
            the total shown. Nothing is rescaled toward the win probability, and the ranges are
            shown as drawn.
          </Typography>
        </Stack>
      ) : (
        <Typography variant="caption" color="text.secondary" data-testid="forecast-note">
          {forecast.note ??
            'No simulated total was served for this fixture, and the answer carried no reason; none is invented here.'}
        </Typography>
      )}
    </Paper>
  );
};

export default MatchScorecard;
