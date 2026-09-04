import React from 'react';
import {
  Chip,
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
import type { MarketBenchmark } from '../types';
import { MetricLabel } from './common/MetricInfo';
import MetricValue from './common/MetricValue';
import { formatShare, formatStat } from '../utils/evaluationReport';

/** The three arms, in the order the reader should compare them. */
const arms = [
  {
    key: 'market',
    label: 'Market (closing price)',
    aucKey: 'market_auc',
    brierKey: 'market_brier',
  },
  {
    key: 'display',
    label: 'Display model, as served',
    aucKey: 'display_auc_mean',
    brierKey: 'display_brier_mean',
  },
  {
    key: 'toss_aware',
    label: 'Display model, toss-aware',
    aucKey: 'display_toss_aware_auc',
    brierKey: 'display_toss_aware_brier',
  },
] as const;

/** "+0.052 (95 % −0.020 to +0.128)", or "—" where there is no gap to state. */
function gapWithInterval(gap?: number, interval?: number[] | null): string {
  if (gap == null) return '—';
  const point = `${gap >= 0 ? '+' : ''}${gap.toFixed(3)}`;
  if (!interval || interval.length !== 2) return point;
  const [low, high] = interval;
  return `${point} (95 % ${low >= 0 ? '+' : ''}${low.toFixed(3)} to ${high >= 0 ? '+' : ''}${high.toFixed(3)})`;
}

/**
 * X-4: how far the displayed probability sits from the betting market's, on the matches
 * both cover.
 *
 * The coverage is printed first and never hidden, because it is half the answer: the
 * cached odds cover two competitions, so most formats have nothing to say here and say so.
 * The gate informs — nothing in the system changes on these numbers, and no model reads
 * odds as an input.
 */
const EvaluationMarketBenchmark: React.FC<{
  benchmark?: MarketBenchmark;
  format: string | null;
}> = ({ benchmark, format }) => {
  if (!benchmark || !format) return null;
  const entry = benchmark.formats[format];
  if (!entry) return null;
  const pooled = entry.pooled;

  return (
    <Paper variant="outlined" sx={{ mb: 3 }}>
      <Stack
        direction="row"
        spacing={1}
        alignItems="center"
        flexWrap="wrap"
        sx={{ px: 2, py: 1.5 }}
      >
        <Typography variant="subtitle1" fontWeight={600}>
          Market benchmark (X-4)
        </Typography>
        <Chip size="small" variant="outlined" label="informs; decides nothing" />
        <Chip
          size="small"
          color={entry.matches_joined > 0 ? 'default' : 'warning'}
          variant="outlined"
          label={`coverage ${formatShare(entry.joined_share)} — ${entry.matches_joined} of ${entry.matches_in_windows} matches`}
        />
        <Chip size="small" variant="outlined" label={`de-vig: ${benchmark.devig.method}`} />
      </Stack>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 2, pb: 1 }}>
        {benchmark.source.name}; {benchmark.source.priced_at}. Joined by {benchmark.join.key}
      </Typography>

      {pooled ? (
        <>
          <TableContainer>
            <Table size="small" aria-label="market benchmark">
              <TableHead>
                <TableRow>
                  <TableCell>Probability source</TableCell>
                  <TableCell align="right">
                    <MetricLabel metricKey="market_auc" label="AUC" />
                  </TableCell>
                  <TableCell align="right">
                    <MetricLabel metricKey="market_brier" label="Brier" />
                  </TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {arms.map((arm) => (
                  <TableRow key={arm.key} hover>
                    <TableCell>{arm.label}</TableCell>
                    <TableCell align="right">
                      <MetricValue metricKey={arm.aucKey} value={pooled[arm.aucKey]} />
                    </TableCell>
                    <TableCell align="right">
                      <MetricValue metricKey={arm.brierKey} value={pooled[arm.brierKey]} />
                    </TableCell>
                  </TableRow>
                ))}
                <TableRow>
                  <TableCell>
                    <MetricLabel
                      metricKey="market_minus_display_auc"
                      label="Market minus display, AUC"
                    />
                  </TableCell>
                  <TableCell align="right" colSpan={2}>
                    {gapWithInterval(
                      pooled.market_minus_display_auc,
                      pooled.market_minus_display_auc_ci95,
                    )}
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>
                    <MetricLabel
                      metricKey="market_minus_toss_aware_auc"
                      label="Market minus display, AUC, toss-aware"
                    />
                  </TableCell>
                  <TableCell align="right" colSpan={2}>
                    {gapWithInterval(
                      pooled.market_minus_toss_aware_auc,
                      pooled.market_minus_toss_aware_auc_ci95,
                    )}
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell>
                    <MetricLabel metricKey="market_brier" label="Market minus display, Brier" />
                  </TableCell>
                  <TableCell align="right" colSpan={2}>
                    {formatStat(pooled.market_minus_display_brier)}
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: 'block', px: 2, py: 1 }}
          >
            Pooled over the walk-forward folds, {pooled.n} matches. {entry.locked.note ?? ''}
          </Typography>
        </>
      ) : (
        <Typography variant="body2" color="text.secondary" sx={{ px: 2, pb: 2 }}>
          No closing price joined to any {format} match in the scored windows, so this format has no
          market comparison. The cached source covers the Big Bash and Women&apos;s Big Bash only;
          internationals need a paid or account-gated feed.
        </Typography>
      )}
    </Paper>
  );
};

export default EvaluationMarketBenchmark;
