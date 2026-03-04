import React from 'react';
import {
  Paper,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from '@mui/material';
import type { PredictTeamSelectedPlayer } from '../types';

export interface TeamTableProps {
  teamName: string;
  players: PredictTeamSelectedPlayer[];
}

const TeamTable: React.FC<TeamTableProps> = ({ teamName, players }) => {
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
              <TableCell align="right">Runs</TableCell>
              <TableCell align="right">Wkts</TableCell>
              <TableCell align="right">Econ</TableCell>
              <TableCell align="right">Catches</TableCell>
              <TableCell align="right">RO</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {players.map((p) => (
              <TableRow key={p.player_id}>
                <TableCell>{p.player_name}</TableCell>
                <TableCell align="right">{p.runs.toFixed(1)}</TableCell>
                <TableCell align="right">{p.wickets.toFixed(1)}</TableCell>
                <TableCell align="right">{p.economy.toFixed(2)}</TableCell>
                <TableCell align="right">{p.catches.toFixed(0)}</TableCell>
                <TableCell align="right">{p.run_outs.toFixed(0)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Paper>
  );
};

export default TeamTable;
