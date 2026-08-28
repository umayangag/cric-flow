import React from 'react';
import {
  Box,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { MatchScorecardResponse } from '../types';

import { formatCount, formatDecimal } from '../utils/format';

type Props = {
  scorecard: MatchScorecardResponse | null;
  loading?: boolean;
  error?: string | null;
  /** Override default "Match summary" title */
  title?: string;
  /** Optional subtitle (e.g. for predicted card: "ML using data before match date") */
  subtitle?: string;
};

const MatchScorecard: React.FC<Props> = ({
  scorecard,
  loading,
  error,
  title = 'Match summary',
  subtitle,
}) => {
  if (loading) {
    return (
      <Typography color="text.secondary" sx={{ fontStyle: 'italic', py: 2 }}>
        Loading match summary…
      </Typography>
    );
  }
  if (error) {
    return (
      <Typography color="error" sx={{ py: 2 }}>
        {error}
      </Typography>
    );
  }
  if (!scorecard || !scorecard.innings?.length) {
    return (
      <Typography color="text.secondary" sx={{ fontStyle: 'italic', py: 2 }}>
        No scorecard available for this match.
      </Typography>
    );
  }

  const dateStr = scorecard.match_date
    ? new Date(scorecard.match_date).toISOString().slice(0, 10)
    : '';

  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="subtitle1" fontWeight="bold" gutterBottom>
        {title}
      </Typography>
      <Typography variant="body2" color="text.secondary" gutterBottom>
        {subtitle ?? `${dateStr}${scorecard.venue ? ` · ${scorecard.venue}` : ''}`}
      </Typography>

      {scorecard.innings.map((inn) => (
        <Paper key={inn.inning_number} sx={{ mt: 2, p: 2 }} variant="outlined">
          <Typography variant="subtitle2" fontWeight="bold" gutterBottom>
            Inning {inn.inning_number}: {inn.batting_team_name} vs {inn.bowling_team_name}
          </Typography>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            {inn.batting_team_name} {inn.runs_scored}/{inn.wickets_lost}
            {inn.extras > 0 ? ` (extras ${inn.extras})` : ''}
            {inn.target_runs != null && inn.target_runs > 0 ? ` · Target ${inn.target_runs}` : ''}
          </Typography>

          <Typography variant="caption" fontWeight="bold" component="div" sx={{ mt: 1 }}>
            Batting — {inn.batting_team_name}
          </Typography>
          <TableContainer>
            <Table size="small" aria-label={`Batting inning ${inn.inning_number}`}>
              <TableHead>
                <TableRow>
                  <TableCell>Batter</TableCell>
                  <TableCell align="right">R</TableCell>
                  <TableCell align="right">B</TableCell>
                  <TableCell align="right">4s</TableCell>
                  <TableCell align="right">6s</TableCell>
                  <TableCell align="right">SR</TableCell>
                  <TableCell>How out</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {inn.batting.map((b, idx) => (
                  <TableRow key={`${b.player_name}-${idx}`}>
                    <TableCell>{b.player_name}</TableCell>
                    <TableCell align="right">{formatCount(b.runs)}</TableCell>
                    <TableCell align="right">{formatCount(b.balls)}</TableCell>
                    <TableCell align="right">{formatCount(b.fours)}</TableCell>
                    <TableCell align="right">{formatCount(b.sixes)}</TableCell>
                    <TableCell align="right">{formatDecimal(b.strike_rate)}</TableCell>
                    <TableCell>{b.how_out ?? '-'}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>

          <Typography variant="caption" fontWeight="bold" component="div" sx={{ mt: 2 }}>
            Bowling — {inn.bowling_team_name}
          </Typography>
          <TableContainer>
            <Table size="small" aria-label={`Bowling inning ${inn.inning_number}`}>
              <TableHead>
                <TableRow>
                  <TableCell>Bowler</TableCell>
                  <TableCell align="right">O</TableCell>
                  <TableCell align="right">M</TableCell>
                  <TableCell align="right">R</TableCell>
                  <TableCell align="right">W</TableCell>
                  <TableCell align="right">Econ</TableCell>
                  <TableCell align="right">Wides</TableCell>
                  <TableCell align="right">No</TableCell>
                  <TableCell align="right">SR</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {inn.bowling.map((w, idx) => {
                  const balls = w.balls ?? 0;
                  const wickets = w.wickets ?? 0;
                  const bowlSR = wickets > 0 && balls > 0 ? balls / wickets : null;
                  return (
                    <TableRow key={`${w.player_name}-${idx}`}>
                      <TableCell>{w.player_name}</TableCell>
                      <TableCell align="right">{formatDecimal(w.overs, 1)}</TableCell>
                      <TableCell align="right">{formatCount(w.maidens)}</TableCell>
                      <TableCell align="right">{formatCount(w.runs)}</TableCell>
                      <TableCell align="right">{formatCount(w.wickets)}</TableCell>
                      <TableCell align="right">{formatDecimal(w.economy)}</TableCell>
                      <TableCell align="right">{formatCount(w.wides)}</TableCell>
                      <TableCell align="right">{formatCount(w.no_balls)}</TableCell>
                      <TableCell align="right">{formatDecimal(bowlSR)}</TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        </Paper>
      ))}
    </Box>
  );
};

export default MatchScorecard;
