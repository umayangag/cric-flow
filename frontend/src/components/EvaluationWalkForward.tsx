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
import type { EvaluationFold, EvaluationFormatReport } from '../types';
import { foldWindow, formatShare, formatStat } from '../utils/evaluationReport';

/**
 * The walk-forward table (H-19): one row per rolling origin, then the locked window.
 *
 * The locked window is labelled and set apart because it is scored once per release and
 * never used for a choice. Showing it in the same table as the folds is the point — the
 * folds are where decisions are made, and the reader should be able to see whether the
 * locked figures sit inside their spread.
 */
const EvaluationWalkForward: React.FC<{ report: EvaluationFormatReport }> = ({ report }) => {
  const { folds, summary } = report.walk_forward;
  const rows: Array<{ fold: EvaluationFold; locked: boolean }> = [
    ...folds.map((fold) => ({ fold, locked: false })),
    { fold: report.locked, locked: true },
  ];

  return (
    <Paper variant="outlined" sx={{ mb: 3 }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ px: 2, py: 1.5 }}>
        <Typography variant="subtitle1" fontWeight={600}>
          Walk-forward
        </Typography>
        <Chip
          size="small"
          variant="outlined"
          label={`${report.n_matches.toLocaleString()} matches`}
        />
      </Stack>
      <TableContainer>
        <Table size="small" aria-label="walk-forward folds">
          <TableHead>
            <TableRow>
              <TableCell>Window</TableCell>
              <TableCell align="right">Train</TableCell>
              <TableCell align="right">Eval</TableCell>
              <TableCell align="right" title="The value the optimiser maximises">
                Objective AUC
              </TableCell>
              <TableCell align="right" title="The probability that is displayed">
                Display AUC
              </TableCell>
              <TableCell align="right">Brier</TableCell>
              <TableCell align="right" title="Brier of always predicting the training base rate">
                Base rate
              </TableCell>
              <TableCell align="right" title="Share of one-player upgrades that lower P(win) (H-4)">
                Swap violations
              </TableCell>
              <TableCell align="right" title="AUC of the specific XI beyond the side's typical XI">
                Specific-XI Δ
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map(({ fold, locked }) => (
              <TableRow key={`${fold.cutoff}-${locked}`} hover selected={locked}>
                <TableCell>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <span>{foldWindow(fold.cutoff, fold.end)}</span>
                    {locked && <Chip size="small" color="primary" label="locked" />}
                  </Stack>
                </TableCell>
                <TableCell align="right">{fold.n_train.toLocaleString()}</TableCell>
                <TableCell align="right">{fold.n_eval.toLocaleString()}</TableCell>
                <TableCell align="right">{formatStat(fold.objective_auc)}</TableCell>
                <TableCell align="right">{formatStat(fold.display_auc_mean)}</TableCell>
                <TableCell align="right">{formatStat(fold.display_brier_mean)}</TableCell>
                <TableCell align="right">{formatStat(fold.base_rate_brier)}</TableCell>
                <TableCell align="right">
                  {formatShare(fold.swap_monotonicity?.violation_share)}
                </TableCell>
                <TableCell align="right">{formatStat(fold.specific_vs_typical?.delta)}</TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell colSpan={3}>
                <Typography variant="body2" fontWeight={600}>
                  Mean over folds
                </Typography>
              </TableCell>
              <TableCell align="right">{formatStat(summary.objective_auc)}</TableCell>
              <TableCell align="right">{formatStat(summary.display_auc)}</TableCell>
              <TableCell align="right">—</TableCell>
              <TableCell align="right">{formatStat(summary.base_rate_brier)}</TableCell>
              <TableCell align="right">{formatShare(summary.swap_violation_share)}</TableCell>
              <TableCell align="right">{formatStat(summary.specific_vs_typical_delta)}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TableContainer>
      {report.locked.recalibrated_targets?.length ? (
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: 'block', px: 2, py: 1 }}
        >
          Recalibrated on a temporal fold before the locked window (H-5):{' '}
          {report.locked.recalibrated_targets.join(', ')}
        </Typography>
      ) : null}
    </Paper>
  );
};

export default EvaluationWalkForward;
