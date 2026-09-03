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
import type { EvaluationPerformance as PerformanceReport } from '../types';
import { MetricLabel } from './common/MetricInfo';
import MetricValue from './common/MetricValue';
import { formatStat } from '../utils/evaluationReport';

/**
 * The performance model per target, never pooled (H-12), with the width of the 10–90
 * interval beside its coverage (H-22).
 *
 * Width beside coverage is the whole point of the pairing: a narrower interval is progress
 * only when coverage holds, and a narrower one that under-covers is a regression. Showing
 * either alone invites reading the wrong one as an improvement.
 */
const targetLabels: Record<string, string> = {
  runs: 'Runs',
  balls_faced: 'Balls faced',
  wickets: 'Wickets',
  runs_conceded: 'Runs conceded',
  catches: 'Catches',
};

const EvaluationPerformanceTable: React.FC<{
  performance: PerformanceReport | null | undefined;
  title: string;
  caption: string;
}> = ({ performance, title, caption }) => {
  const targets = performance?.targets;
  if (!targets || !Object.keys(targets).length) {
    return (
      <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
        <Typography variant="subtitle1" fontWeight={600} gutterBottom>
          {title}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {performance?.skipped_reason ?? 'No performance model was scored for this format.'}
        </Typography>
      </Paper>
    );
  }

  return (
    <Paper variant="outlined" sx={{ mb: 3 }}>
      <Typography variant="subtitle1" fontWeight={600} sx={{ px: 2, py: 1.5 }}>
        {title}
      </Typography>
      <TableContainer>
        <Table size="small" aria-label={title}>
          <TableHead>
            <TableRow>
              <TableCell>Target</TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="within_match_spearman" label="Spearman" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="top3_hit_rate" label="Top-3 hit" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="mae" label="MAE" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="pinball" label="Pinball" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="coverage_80" label="10–90 coverage" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="width_80" label="10–90 width" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="within_match_spearman" label="Career mean Spearman" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="pinball" label="Career mean pinball" />
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.entries(targets).map(([name, entry]) => (
              <TableRow key={name} hover>
                <TableCell>{targetLabels[name] ?? name}</TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="within_match_spearman"
                    value={entry.model?.within_match_spearman}
                  />
                </TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="top3_hit_rate"
                    value={entry.model?.top3_hit_rate}
                    baseline={entry.career_mean?.top3_hit_rate}
                    as="share"
                  />
                </TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="mae"
                    value={entry.model?.mae}
                    baseline={entry.career_mean?.mae}
                    digits={2}
                  />
                </TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="pinball"
                    value={entry.model?.pinball}
                    baseline={entry.career_mean?.pinball}
                  />
                </TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="coverage_80"
                    value={entry.model?.interval?.coverage_80}
                    as="share"
                  />
                </TableCell>
                <TableCell align="right">
                  {formatStat(entry.model?.interval?.width_80, 1)}
                </TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="within_match_spearman"
                    value={entry.career_mean?.within_match_spearman}
                  />
                </TableCell>
                <TableCell align="right">{formatStat(entry.career_mean?.pinball, 3)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 2, py: 1 }}>
        {caption}
      </Typography>
    </Paper>
  );
};

export default EvaluationPerformanceTable;
