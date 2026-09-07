import React from 'react';
import {
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { TrackRecordEntry, TrackRecordRange } from '../../types';
import { MetricLabel } from '../common/MetricInfo';

export const STATE_LABELS: Record<TrackRecordEntry['state'], string> = {
  scenario: 'scenario',
  superseded: 'superseded',
  unresolved: 'unresolved',
  no_result: 'no result',
  scored: 'scored',
};

export const POPULATION_LABELS: Record<TrackRecordEntry['population'], string> = {
  with_shared_factor: 'shared factor',
  without_shared_factor: 'no shared factor',
  unknown: 'unknown simulator',
  not_simulated: 'not simulated',
};

function range(served?: TrackRecordRange): string {
  return served ? `${Math.round(served.p10)}–${Math.round(served.p90)}` : '—';
}

/** What the state means for this row, in a phrase: never a verdict. */
function stateDetail(entry: TrackRecordEntry): string {
  switch (entry.state) {
    case 'unresolved': {
      const days = entry.days_past_match_date ?? 0;
      if (entry.state_note) return entry.state_note;
      if (days < 0) return `match in ${-days} day${days === -1 ? '' : 's'}`;
      if (days === 0) return 'match day; not imported yet';
      return `${days} day${days === 1 ? '' : 's'} past the match date; not imported yet`;
    }
    case 'superseded':
      return entry.state_note ?? 'a later forecast of this fixture replaced it';
    case 'scenario':
      return 'hand-built eleven; listed, never scored';
    case 'no_result':
      return 'played, no winner recorded';
    case 'scored':
      return entry.score?.team1_won ? `${entry.team1.name} won` : `${entry.team2.name} won`;
  }
}

/** The actual totals in the prediction's orientation, with the batting order. */
function happenedTotals(entry: TrackRecordEntry): string {
  const h = entry.happened;
  if (!h) return '—';
  const one = h.team1_total ?? '—';
  const two = h.team2_total ?? '—';
  const order =
    h.team1_batted_first == null
      ? ''
      : h.team1_batted_first
        ? ' (team1 batted first)'
        : ' (team2 batted first)';
  return `${one} / ${two}${order}`;
}

function coveredMark(hit: boolean | null | undefined): string {
  if (hit == null) return '—';
  return hit ? 'in' : 'out';
}

/**
 * Every stored prediction, newest first, in its state: what was claimed, what happened,
 * the eleven overlap. Nothing here is filtered by outcome and nothing is coloured by
 * threshold -- a miss is written out the same as a hit.
 */
const TrackRecordList: React.FC<{ predictions: TrackRecordEntry[] }> = ({ predictions }) => (
  <TableContainer>
    <Table size="small" aria-label="track record predictions">
      <TableHead>
        <TableRow>
          <TableCell>Issued</TableCell>
          <TableCell>Fixture</TableCell>
          <TableCell>State</TableCell>
          <TableCell align="right">Claimed P(team1)</TableCell>
          <TableCell align="right">Claimed totals (10–90)</TableCell>
          <TableCell align="right">Actual totals</TableCell>
          <TableCell align="right">
            <MetricLabel metricKey="coverage_80" label="In range" />
          </TableCell>
          <TableCell align="right">
            <MetricLabel metricKey="brier" label="Brier" />
          </TableCell>
          <TableCell align="right">
            <MetricLabel metricKey="eleven_overlap" label="Eleven overlap" />
          </TableCell>
          <TableCell>Simulator</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {predictions.map((entry) => (
          <TableRow key={entry.id} hover data-testid="track-record-row" data-state={entry.state}>
            <TableCell>
              <Typography variant="body2">
                {entry.issued_at.slice(0, 16).replace('T', ' ')}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                run {entry.run_id} · through {entry.ratings_through}
              </Typography>
            </TableCell>
            <TableCell>
              <Typography variant="body2">
                {entry.team1.name || entry.team1.id} v {entry.team2.name || entry.team2.id}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {entry.format} · {entry.match_date} · {entry.objective}
                {entry.issued_after_match_date ? ' · issued after the match date' : ''}
              </Typography>
            </TableCell>
            <TableCell>
              <Chip size="small" variant="outlined" label={STATE_LABELS[entry.state]} />
              <Typography variant="caption" color="text.secondary" display="block">
                {stateDetail(entry)}
              </Typography>
              {entry.payload_error && (
                <Typography variant="caption" color="warning.main" display="block">
                  {entry.payload_error}
                </Typography>
              )}
            </TableCell>
            <TableCell align="right">
              {entry.claimed.win_probability_team1.toFixed(3)}
              <Typography variant="caption" color="text.secondary" display="block">
                {entry.claimed.win_probability_source}
              </Typography>
            </TableCell>
            <TableCell align="right">
              {range(entry.claimed.team1_range)} / {range(entry.claimed.team2_range)}
            </TableCell>
            <TableCell align="right">{happenedTotals(entry)}</TableCell>
            <TableCell align="right">
              {entry.score
                ? `${coveredMark(entry.score.team1_covered)} / ${coveredMark(entry.score.team2_covered)}`
                : '—'}
            </TableCell>
            <TableCell align="right">{entry.score ? entry.score.brier.toFixed(3) : '—'}</TableCell>
            <TableCell align="right">
              {entry.score
                ? `${entry.score.eleven_overlap.matched} of ${entry.score.eleven_overlap.of}`
                : `${entry.claimed.team1_players + entry.claimed.team2_players} named`}
            </TableCell>
            <TableCell>{POPULATION_LABELS[entry.population]}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </TableContainer>
);

export default TrackRecordList;
