import React, { useMemo, useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Chip,
  Collapse,
  Grid,
  IconButton,
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
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import type { BacktestEvaluateResponse } from '../types';

/** Default CV for assumed distribution (runs ~0.35, wickets ~0.4, economy ~0.15). */
const DEFAULT_CV: Record<string, number> = {
  runs: 0.35,
  wickets: 0.4,
  economy: 0.15,
  catches: 0.5,
  run_outs: 0.5,
};

function assumedPercentiles(
  mean: number,
  metricKey: string,
): { p10: number; p50: number; p90: number } {
  const cv = DEFAULT_CV[metricKey] ?? 0.35;
  const sigma = Math.max(mean * cv, 0.1);
  const z = 1.28; // ~80% interval
  return {
    p10: Math.max(0, mean - z * sigma),
    p50: mean,
    p90: mean + z * sigma,
  };
}

const PredictedCellWithDistribution: React.FC<{
  playerId: number;
  metricKey: string;
  predVal: number | undefined;
  expandedKey: string | null;
  onToggle: (key: string) => void;
}> = ({ playerId, metricKey, predVal, expandedKey, onToggle }) => {
  const key = `${playerId}-${metricKey}`;
  const expanded = expandedKey === key;
  const numVal = typeof predVal === 'number' && Number.isFinite(predVal) ? predVal : null;
  const percentiles = numVal != null ? assumedPercentiles(numVal, metricKey) : null;

  return (
    <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25 }}>
      <Typography component="span" variant="body2">
        {formatCell(predVal)}
      </Typography>
      {numVal != null && (
        <IconButton
          size="small"
          aria-label={expanded ? 'Hide distribution' : 'Show distribution'}
          onClick={() => onToggle(key)}
          sx={{ p: 0.25 }}
        >
          {expanded ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
        </IconButton>
      )}
      {expanded && percentiles != null && (
        <Collapse in={expanded}>
          <Paper variant="outlined" sx={{ p: 1, mt: 0.5, bgcolor: 'grey.50' }}>
            <Typography variant="caption" color="text.secondary" display="block">
              Assumed distribution (CV={DEFAULT_CV[metricKey] ?? 0.35})
            </Typography>
            <Typography variant="caption" component="div">
              P10: {percentiles.p10.toFixed(1)} · P50: {percentiles.p50.toFixed(1)} · P90:{' '}
              {percentiles.p90.toFixed(1)}
            </Typography>
            <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 0.5 }}>
              Full simulation distribution can be added via API.
            </Typography>
          </Paper>
        </Collapse>
      )}
    </Box>
  );
};

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

function diffSeverity(
  value: number | undefined,
  thresholds: { low: number; high: number },
): 'good' | 'moderate' | 'high' | 'none' {
  if (value == null || typeof value !== 'number' || !Number.isFinite(value)) return 'none';
  const { low, high } = thresholds;
  if (value <= low) return 'good';
  if (value <= high) return 'moderate';
  return 'high';
}

const PlayersTable: React.FC<{ result: BacktestEvaluateResponse }> = ({ result }) => {
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
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
    {
      key: 'wickets',
      label: 'Wkts',
      maeKey: 'wickets_mae',
      enabled: anyWickets,
      thresholds: { low: 0.5, high: 1.5 },
    },
    {
      key: 'economy',
      label: 'Econ',
      maeKey: 'economy_mae',
      enabled: anyEconomy,
      thresholds: { low: 0.5, high: 1.5 },
    },
    {
      key: 'catches',
      label: 'Catches',
      maeKey: 'catches_mae',
      enabled: anyCatches,
      thresholds: { low: 0.5, high: 1 },
    },
    {
      key: 'run_outs',
      label: 'Run Outs',
      maeKey: 'run_outs_mae',
      enabled: anyRunOuts,
      thresholds: { low: 0.5, high: 1 },
    },
  ].filter((m) => m.enabled);

  const allMetrics = [
    { key: 'runs', label: 'Runs', maeKey: 'runs_mae', thresholds: { low: 5, high: 15 } },
    ...optionalMetrics,
  ];

  return (
    <TableContainer component={Paper} sx={{ maxHeight: 420 }}>
      <Table stickyHeader size="small" aria-label="players actual vs predicted comparison">
        <TableHead>
          <TableRow>
            <TableCell sx={{ fontWeight: 600 }}>Player</TableCell>
            {allMetrics.map((m) => (
              <React.Fragment key={m.key}>
                <TableCell
                  align="right"
                  sx={{ bgcolor: 'action.hover', fontWeight: 600, minWidth: 56 }}
                >
                  {m.label} Actual
                </TableCell>
                <TableCell
                  align="right"
                  sx={{
                    bgcolor: 'primary.light',
                    color: 'primary.dark',
                    fontWeight: 600,
                    minWidth: 56,
                  }}
                >
                  {m.label} Pred
                </TableCell>
                <TableCell
                  align="center"
                  sx={{ fontWeight: 600, minWidth: 44 }}
                  title="Difference (|Pred - Actual|)"
                >
                  Δ
                </TableCell>
              </React.Fragment>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {result.players.map((p) => (
            <TableRow key={p.player_id} hover>
              <TableCell component="th" scope="row" sx={{ fontWeight: 500 }}>
                {p.player_id}
              </TableCell>
              {allMetrics.map((m) => {
                const actualVal = p.actual[m.key];
                const predVal = p.predicted[m.key];
                const err = p.errors[m.maeKey];
                const severity = diffSeverity(
                  typeof err === 'number' ? err : undefined,
                  m.thresholds,
                );
                return (
                  <React.Fragment key={m.key}>
                    <TableCell align="right" sx={{ bgcolor: 'action.hover' }}>
                      {formatCell(actualVal)}
                    </TableCell>
                    <TableCell
                      align="right"
                      sx={{ bgcolor: 'primary.light', color: 'primary.dark', position: 'relative' }}
                    >
                      <PredictedCellWithDistribution
                        playerId={p.player_id}
                        metricKey={m.key}
                        predVal={typeof predVal === 'number' ? predVal : undefined}
                        expandedKey={expandedKey}
                        onToggle={(key) => setExpandedKey((prev) => (prev === key ? null : key))}
                      />
                    </TableCell>
                    <TableCell
                      align="center"
                      sx={{
                        fontWeight: 600,
                        ...(severity === 'good' && {
                          color: 'success.main',
                          bgcolor: 'success.light',
                          opacity: 0.9,
                        }),
                        ...(severity === 'moderate' && {
                          color: 'warning.dark',
                          bgcolor: 'warning.light',
                          opacity: 0.9,
                        }),
                        ...(severity === 'high' && {
                          color: 'error.contrastText',
                          bgcolor: 'error.main',
                          opacity: 0.9,
                        }),
                      }}
                    >
                      {formatCell(err)}
                    </TableCell>
                  </React.Fragment>
                );
              })}
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
        <strong>{new Date(result.match.match_date).toISOString().slice(0, 10)}</strong>
      </Typography>
      <MetricsLine metrics={result.metrics} />
      <MatchAggregates aggregates={result.match_aggregates} />
      <Typography variant="subtitle2" fontWeight={600} gutterBottom sx={{ mt: 2 }}>
        Player performance: Actual vs Predicted
      </Typography>
      <Typography variant="caption" color="text.secondary" component="div" sx={{ mb: 1 }}>
        Each metric shows Actual | Predicted | Δ (absolute error). Δ is color-coded: green = small,
        amber = moderate, red = large.
      </Typography>
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
