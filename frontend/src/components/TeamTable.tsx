import React from 'react';
import {
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { MetricLabel } from './common/MetricInfo';
import type { PredictTeamSelectedPlayer, PredictValueRange } from '../types';

export interface TeamTableProps {
  teamName: string;
  players: PredictTeamSelectedPlayer[];
  /** False for a rating-ordered XI: no player has a marginal value, so the column is hidden. */
  optimised: boolean;
}

/** "23 (4–55)" — the point the scorecard shows, and the range the model actually gave. */
function pointWithRange(
  value: number | undefined,
  range: PredictValueRange | undefined,
  digits = 0,
): string {
  if (value == null) return '—';
  const point = value.toFixed(digits);
  if (!range) return point;
  return `${point} (${range.p10.toFixed(digits)}–${range.p90.toFixed(digits)})`;
}

/**
 * One side's XI.
 *
 * Every point carries its 10–90 range, because that is what the model produced: a median
 * with no interval reads as a promise, and the plan's whole answer to "performance
 * prediction is hard" is to show the spread rather than hide it. The marginal value is
 * what the XI loses if the player is replaced by an average one — the L3 explanation of
 * why he is in it.
 */
const TeamTable: React.FC<TeamTableProps> = ({ teamName, players, optimised }) => {
  return (
    <Paper variant="outlined" sx={{ flex: 1, overflow: 'hidden' }}>
      <Typography variant="subtitle2" sx={{ px: 2, py: 1, bgcolor: 'action.hover' }}>
        {teamName}
      </Typography>
      <TableContainer>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Player</TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="range_10_90" label="Runs (10–90)" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="range_10_90" label="Balls (10–90)" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="range_10_90" label="Wkts (10–90)" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="range_10_90" label="Conceded (10–90)" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="economy" label="Econ" />
              </TableCell>
              {optimised && (
                <TableCell align="right">
                  <MetricLabel metricKey="marginal_value" label="Marginal" />
                </TableCell>
              )}
            </TableRow>
          </TableHead>
          <TableBody>
            {players.map((p) => (
              <TableRow key={p.player_id}>
                <TableCell>{p.player_name}</TableCell>
                <TableCell align="right">{pointWithRange(p.runs, p.runs_range)}</TableCell>
                <TableCell align="right">{pointWithRange(p.balls, p.balls_range)}</TableCell>
                <TableCell align="right">{pointWithRange(p.wickets, p.wickets_range, 1)}</TableCell>
                <TableCell align="right">
                  {pointWithRange(p.runs_conceded, p.runs_conceded_range)}
                </TableCell>
                <TableCell align="right">{p.economy ? p.economy.toFixed(2) : '—'}</TableCell>
                {optimised && (
                  <TableCell align="right">
                    {p.marginal_value == null ? '—' : `${(p.marginal_value * 100).toFixed(1)} pp`}
                  </TableCell>
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Paper>
  );
};

export default TeamTable;
