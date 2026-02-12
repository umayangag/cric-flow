import React from 'react';
import {
  Paper,
  Radio,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { BacktestCandidate } from '../types';

type CandidatesTableProps = {
  candidates: BacktestCandidate[];
  selectedMatchId: number | null;
  onSelectMatch: (matchId: number) => void;
};

const CandidatesTable: React.FC<CandidatesTableProps> = ({
  candidates,
  selectedMatchId,
  onSelectMatch,
}) => {
  if (!candidates.length) {
    return (
      <Typography color="text.secondary" sx={{ fontStyle: 'italic' }}>
        No candidates loaded yet.
      </Typography>
    );
  }
  return (
    <TableContainer component={Paper} sx={{ maxHeight: 400 }}>
      <Table stickyHeader size="small" aria-label="candidates table">
        <TableHead>
          <TableRow>
            <TableCell>Date</TableCell>
            <TableCell>Match</TableCell>
            <TableCell>Venue</TableCell>
            <TableCell>Winner</TableCell>
            <TableCell align="center">Select</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {candidates.map((c) => (
            <TableRow
              key={c.match_id}
              hover
              selected={selectedMatchId === c.match_id}
              onClick={() => onSelectMatch(c.match_id)}
              sx={{ cursor: 'pointer' }}
            >
              <TableCell>{new Date(c.match_date).toISOString().slice(0, 10)}</TableCell>
              <TableCell>
                {c.team1} vs {c.team2}
              </TableCell>
              <TableCell>{c.venue || '-'}</TableCell>
              <TableCell>{c.winner_team_code || '-'}</TableCell>
              <TableCell align="center">
                <Radio
                  checked={selectedMatchId === c.match_id}
                  onChange={() => onSelectMatch(c.match_id)}
                  onClick={(e) => e.stopPropagation()}
                  value={c.match_id}
                  name="candidate-radio"
                  size="small"
                  inputProps={{ 'aria-label': 'Select' }}
                />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

export default CandidatesTable;
