import React, { useState } from 'react';
import {
  Collapse,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import {
  KeyboardArrowDown as KeyboardArrowDownIcon,
  KeyboardArrowUp as KeyboardArrowUpIcon,
} from '@mui/icons-material';
import { MetricLabel } from './common/MetricInfo';
import WhyThisPlayer from './WhyThisPlayer';
import { pointWithRange } from '../utils/format';
import type { PredictSelectionSummary, PredictTeamSelectedPlayer } from '../types';

export interface TeamTableProps {
  teamName: string;
  players: PredictTeamSelectedPlayer[];
  /**
   * How this eleven was chosen. The marginal column is hidden where nothing was maximised,
   * and the "why this player" card renders the state that matches (P1-3).
   */
  selection: PredictSelectionSummary;
}

/** The column count, so the card's row spans the whole table whichever columns are shown. */
function columnCount(optimised: boolean): number {
  return optimised ? 8 : 7;
}

/**
 * One side's XI.
 *
 * Every point carries its 10–90 range, because that is what the model produced: a median
 * with no interval reads as a promise, and the plan's whole answer to "performance
 * prediction is hard" is to show the spread rather than hide it. The marginal value is
 * what the XI loses if the player is replaced by an average one — the L3 explanation of
 * why he is in it.
 *
 * Every row opens onto the "why this player" card (P1-3), which shows what the selection
 * itself read about him and nothing else.
 */
const TeamTable: React.FC<TeamTableProps> = ({ teamName, players, selection }) => {
  const [openPlayerID, setOpenPlayerID] = useState<number | null>(null);
  const optimised = selection.optimised;
  return (
    <Paper variant="outlined" sx={{ flex: 1, overflow: 'hidden' }}>
      <Typography variant="subtitle2" sx={{ px: 2, py: 1, bgcolor: 'action.hover' }}>
        {teamName}
      </Typography>
      <TableContainer>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell sx={{ width: 40 }} />
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
              <React.Fragment key={p.player_id}>
                <TableRow>
                  <TableCell sx={{ borderBottom: 'none' }}>
                    <IconButton
                      size="small"
                      aria-label={`Why ${p.player_name}?`}
                      aria-expanded={openPlayerID === p.player_id}
                      onClick={() =>
                        setOpenPlayerID(openPlayerID === p.player_id ? null : p.player_id)
                      }
                    >
                      {openPlayerID === p.player_id ? (
                        <KeyboardArrowUpIcon fontSize="small" />
                      ) : (
                        <KeyboardArrowDownIcon fontSize="small" />
                      )}
                    </IconButton>
                  </TableCell>
                  <TableCell>{p.player_name}</TableCell>
                  <TableCell align="right">{pointWithRange(p.runs, p.runs_range)}</TableCell>
                  <TableCell align="right">{pointWithRange(p.balls, p.balls_range)}</TableCell>
                  <TableCell align="right">
                    {pointWithRange(p.wickets, p.wickets_range, 1)}
                  </TableCell>
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
                <TableRow>
                  <TableCell sx={{ py: 0 }} colSpan={columnCount(optimised)}>
                    <Collapse in={openPlayerID === p.player_id} unmountOnExit>
                      <WhyThisPlayer player={p} selection={selection} />
                    </Collapse>
                  </TableCell>
                </TableRow>
              </React.Fragment>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Paper>
  );
};

export default TeamTable;
