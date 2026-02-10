import React, { useMemo } from 'react';
import {
  Box,
  Card,
  CardContent,
  Chip,
  Grid,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import type { BacktestEvaluateResponse } from '../types';

type EvaluationResultsProps = {
  result: BacktestEvaluateResponse;
};

const MetricsLine: React.FC<{
  metrics: Record<string, number> | undefined;
}> = ({ metrics }) => {
  const parts = useMemo(() => {
    if (!metrics) return [] as string[];
    const order = [
      'player_runs_mae',
      'player_runs_rmse',
      'player_runs_r2',
      'player_wickets_mae',
      'player_economy_mae',
      'player_catches_mae',
      'player_run_outs_mae',
      'match_runs_mae',
      'match_wickets_mae',
      'match_extras_mae',
      'winner_accuracy',
    ];
    return order
      .filter((k) => typeof metrics[k] === 'number')
      .map((k) => `${k} = ${formatMetric(metrics[k])}`);
  }, [metrics]);

  if (!parts.length) return null;
  return (
    <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ mb: 2 }}>
      {parts.map((p, i) => (
        <Chip key={i} label={p} variant="outlined" size="small" sx={{ mb: 1 }} />
      ))}
    </Stack>
  );
};

function formatMetric(v: number | undefined): string {
  if (typeof v !== 'number') return '-';
  const isIntLike = Math.abs(v - Math.round(v)) < 1e-9;
  return isIntLike ? String(Math.round(v)) : v.toFixed(3);
}

const MatchAggregates: React.FC<{
  aggregates: BacktestEvaluateResponse['match_aggregates'];
}> = ({ aggregates }) => {
  if (!aggregates) return null;
  return (
    <Card variant="outlined" sx={{ mb: 2 }}>
      <CardContent>
        <Typography variant="subtitle1" fontWeight={600} gutterBottom>
          Match aggregates
        </Typography>
        <Grid container spacing={4}>
          <Grid item xs={12} sm={4}>
            <Typography variant="subtitle2" fontWeight={600} color="primary">
              Predicted
            </Typography>
            <Typography variant="body2">
              runs: <strong>{String(aggregates.predicted?.runs ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              wickets: <strong>{String(aggregates.predicted?.wickets ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              extras: <strong>{String(aggregates.predicted?.extras ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              winner: <strong>{String(aggregates.predicted?.winner_team_code ?? '-')}</strong>
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4}>
            <Typography variant="subtitle2" fontWeight={600} color="secondary">
              Actual
            </Typography>
            <Typography variant="body2">
              runs: <strong>{String(aggregates.actual?.runs ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              wickets: <strong>{String(aggregates.actual?.wickets ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              extras: <strong>{String(aggregates.actual?.extras ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              winner: <strong>{String(aggregates.actual?.winner_team_code ?? '-')}</strong>
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4}>
            <Typography variant="subtitle2" fontWeight={600} color="error">
              Errors
            </Typography>
            <Typography variant="body2">
              runs_mae: <strong>{String(aggregates.errors?.runs_mae ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              wickets_mae: <strong>{String(aggregates.errors?.wickets_mae ?? '-')}</strong>
            </Typography>
            <Typography variant="body2">
              extras_mae: <strong>{String(aggregates.errors?.extras_mae ?? '-')}</strong>
            </Typography>
          </Grid>
        </Grid>
      </CardContent>
    </Card>
  );
};

const PlayersTable: React.FC<{ result: BacktestEvaluateResponse }> = ({ result }) => {
  const anyWickets = result.players?.some(
    (p) => typeof p.predicted['wickets'] === 'number' || typeof p.actual['wickets'] === 'number',
  );
  const anyEconomy = result.players?.some(
    (p) => typeof p.predicted['economy'] === 'number' || typeof p.actual['economy'] === 'number',
  );
  const anyCatches = result.players?.some(
    (p) => typeof p.predicted['catches'] === 'number' || typeof p.actual['catches'] === 'number',
  );
  const anyRunOuts = result.players?.some(
    (p) => typeof p.predicted['run_outs'] === 'number' || typeof p.actual['run_outs'] === 'number',
  );

  const optionalMetrics = [
    { key: 'wickets', label: 'Wkts', maeKey: 'wickets_mae', enabled: anyWickets },
    { key: 'economy', label: 'Econ', maeKey: 'economy_mae', enabled: anyEconomy },
    { key: 'catches', label: 'Catches', maeKey: 'catches_mae', enabled: anyCatches },
    { key: 'run_outs', label: 'Run Outs', maeKey: 'run_outs_mae', enabled: anyRunOuts },
  ].filter((m) => m.enabled);

  return (
    <TableContainer component={Paper} sx={{ maxHeight: 320 }}>
      <Table stickyHeader size="small" aria-label="players results table">
        <TableHead>
          <TableRow>
            <TableCell>Player ID</TableCell>
            <TableCell align="right">Pred Runs</TableCell>
            <TableCell align="right">Actual Runs</TableCell>
            <TableCell align="right">Abs Error</TableCell>
            {optionalMetrics.map((m) => (
              <React.Fragment key={m.key}>
                <TableCell align="right">Pred {m.label}</TableCell>
                <TableCell align="right">Actual {m.label}</TableCell>
                <TableCell align="right">{m.label} Abs Err</TableCell>
              </React.Fragment>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {result.players.map((p) => (
            <TableRow key={p.player_id} hover>
              <TableCell>{p.player_id}</TableCell>
              <TableCell align="right">{formatCell(p.predicted['runs'])}</TableCell>
              <TableCell align="right">{formatCell(p.actual['runs'])}</TableCell>
              <TableCell align="right">{formatCell(p.errors['runs_mae'])}</TableCell>
              {optionalMetrics.map((m) => (
                <React.Fragment key={m.key}>
                  <TableCell align="right">{formatCell(p.predicted[m.key])}</TableCell>
                  <TableCell align="right">{formatCell(p.actual[m.key])}</TableCell>
                  <TableCell align="right">{formatCell(p.errors[m.maeKey])}</TableCell>
                </React.Fragment>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

const EvaluationResults: React.FC<EvaluationResultsProps> = ({ result }) => {
  return (
    <Box>
      <Typography variant="body2" sx={{ mb: 2 }}>
        Match: <strong>{result.match.match_id}</strong> · Date:{' '}
        <strong>{new Date(result.match.date).toISOString().slice(0, 10)}</strong>
      </Typography>
      <MetricsLine metrics={result.metrics} />
      <MatchAggregates aggregates={result.match_aggregates} />
      <PlayersTable result={result} />
    </Box>
  );
};

function formatCell(v: unknown): string | number {
  if (v == null) return '';
  if (typeof v === 'number') {
    return Number.isInteger(v) ? v : Number.isFinite(v) ? v.toFixed(2) : '';
  }
  return String(v);
}

export default EvaluationResults;
