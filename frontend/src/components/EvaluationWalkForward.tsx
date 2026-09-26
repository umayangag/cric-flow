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
import { MetricLabel } from './common/MetricInfo';
import MetricValue from './common/MetricValue';
import {
  foldProvenance,
  foldWindow,
  formatShare,
  formatStat,
  holdoutSeason,
} from '../utils/evaluationReport';

/**
 * The walk-forward table (H-19): one row per rolling origin, then the locked window.
 *
 * The locked window is labelled and set apart because it is scored once per release and
 * never used for a choice. Showing it in the same table as the folds is the point — the
 * folds are where decisions are made, and the reader should be able to see whether the
 * locked figures sit inside their spread. The two surfaces are named where the numbers
 * are (EVAL-11): the mean row says how many gates have read the folds, and the holdout
 * row says how much of its season has accrued and that no gate read it.
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
              <TableCell align="right">
                <MetricLabel
                  metricKey="objective_auc"
                  label="Objective AUC"
                  value={formatStat(summary.objective_auc)}
                />
              </TableCell>
              <TableCell align="right">
                <MetricLabel
                  metricKey="display_auc_mean"
                  label="Display AUC"
                  value={formatStat(summary.display_auc)}
                />
              </TableCell>
              <TableCell align="right">
                <MetricLabel metricKey="display_brier_mean" label="Brier" />
              </TableCell>
              <TableCell align="right">
                <MetricLabel
                  metricKey="base_rate_brier"
                  label="Base rate"
                  value={formatStat(summary.base_rate_brier)}
                />
              </TableCell>
              <TableCell align="right">
                <MetricLabel
                  metricKey="swap_violation_share"
                  label="Swap violations"
                  value={formatShare(summary.swap_violation_share)}
                />
              </TableCell>
              <TableCell align="right">
                <MetricLabel
                  metricKey="specific_vs_typical_delta"
                  label="Specific-XI Δ"
                  value={formatStat(summary.specific_vs_typical_delta)}
                />
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map(({ fold, locked }) => (
              <TableRow key={`${fold.cutoff}-${locked}`} hover selected={locked}>
                <TableCell>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <span>{foldWindow(fold.cutoff, fold.end)}</span>
                    {locked && <Chip size="small" color="primary" label="holdout" />}
                    {locked && fold.holdout && (
                      <Typography variant="caption" color="text.secondary">
                        {holdoutSeason(fold.holdout)}
                      </Typography>
                    )}
                  </Stack>
                </TableCell>
                <TableCell align="right">{fold.n_train.toLocaleString()}</TableCell>
                <TableCell align="right">{fold.n_eval.toLocaleString()}</TableCell>
                <TableCell align="right">
                  <MetricValue metricKey="objective_auc" value={fold.objective_auc} />
                </TableCell>
                <TableCell align="right">
                  <MetricValue metricKey="display_auc_mean" value={fold.display_auc_mean} />
                </TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="display_brier_mean"
                    value={fold.display_brier_mean}
                    baseline={fold.base_rate_brier}
                  />
                </TableCell>
                <TableCell align="right">{formatStat(fold.base_rate_brier)}</TableCell>
                <TableCell align="right">
                  <MetricValue
                    metricKey="violation_share"
                    value={fold.swap_monotonicity?.violation_share}
                    as="share"
                  />
                </TableCell>
                <TableCell align="right">
                  <MetricValue metricKey="delta" value={fold.specific_vs_typical?.delta} />
                </TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell colSpan={3}>
                <Typography variant="body2" fontWeight={600}>
                  Mean over folds — development surface
                </Typography>
                {foldProvenance(summary.objective_auc) && (
                  <Typography variant="caption" color="text.secondary">
                    {foldProvenance(summary.objective_auc)}
                  </Typography>
                )}
              </TableCell>
              <TableCell align="right">
                <MetricValue metricKey="objective_auc" value={summary.objective_auc} />
              </TableCell>
              <TableCell align="right">
                <MetricValue metricKey="display_auc" value={summary.display_auc} />
              </TableCell>
              <TableCell align="right">—</TableCell>
              <TableCell align="right">{formatStat(summary.base_rate_brier)}</TableCell>
              <TableCell align="right">
                <MetricValue
                  metricKey="swap_violation_share"
                  value={summary.swap_violation_share}
                  as="share"
                />
              </TableCell>
              <TableCell align="right">
                <MetricValue
                  metricKey="specific_vs_typical_delta"
                  value={summary.specific_vs_typical_delta}
                />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TableContainer>
      {report.display_regression && (
        <Typography
          variant="caption"
          color={report.display_regression.verdict === 'fail' ? 'error' : 'text.secondary'}
          data-verdict={report.display_regression.verdict}
          sx={{ display: 'block', px: 2, py: 1 }}
        >
          Display AUC against the previous accepted run (display-regression):{' '}
          {report.display_regression.verdict} — {report.display_regression.reason}
        </Typography>
      )}
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
