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
import type { EvaluationPerformance as PerformanceReport, EvaluationTargetScore } from '../types';
import { formatShare, formatStat } from '../utils/evaluationReport';

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

function coverage(score: EvaluationTargetScore | undefined): string {
  return formatShare(score?.interval?.coverage_80);
}

function width(score: EvaluationTargetScore | undefined): string {
  return formatStat(score?.interval?.width_80, 1);
}

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
              <TableCell align="right" title="Within-match rank correlation with what happened">
                Spearman
              </TableCell>
              <TableCell align="right">Top-3 hit</TableCell>
              <TableCell align="right" title="Absolute error of the median">
                MAE
              </TableCell>
              <TableCell align="right" title="The proper score for a quantile forecast">
                Pinball
              </TableCell>
              <TableCell align="right">10–90 coverage</TableCell>
              <TableCell align="right" title="Narrower is progress only while coverage holds">
                10–90 width
              </TableCell>
              <TableCell align="right" title="The unconditional career mean on the same rows">
                Career mean Spearman
              </TableCell>
              <TableCell align="right">Career mean pinball</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.entries(targets).map(([name, entry]) => (
              <TableRow key={name} hover>
                <TableCell>{targetLabels[name] ?? name}</TableCell>
                <TableCell align="right">
                  {formatStat(entry.model?.within_match_spearman)}
                </TableCell>
                <TableCell align="right">{formatShare(entry.model?.top3_hit_rate)}</TableCell>
                <TableCell align="right">{formatStat(entry.model?.mae, 2)}</TableCell>
                <TableCell align="right">{formatStat(entry.model?.pinball, 3)}</TableCell>
                <TableCell align="right">{coverage(entry.model)}</TableCell>
                <TableCell align="right">{width(entry.model)}</TableCell>
                <TableCell align="right">
                  {formatStat(entry.career_mean?.within_match_spearman)}
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
