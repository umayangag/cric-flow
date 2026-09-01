import React from 'react';
import { Alert, Grid, Paper, Stack, Typography } from '@mui/material';
import type { EvaluationFormatReport } from '../types';
import { formatShare, formatStat, statMean } from '../utils/evaluationReport';

const Metric: React.FC<{ label: string; value: string; caption: string }> = ({
  label,
  value,
  caption,
}) => (
  <Grid item xs={12} md={4}>
    <Paper variant="outlined" sx={{ p: 2, height: '100%' }}>
      <Typography variant="overline" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="h5" sx={{ my: 0.5 }}>
        {value}
      </Typography>
      <Typography variant="caption" color="text.secondary">
        {caption}
      </Typography>
    </Paper>
  </Grid>
);

/**
 * The two selection gates, and the slot for the third.
 *
 * P-0's winner accuracy is not here on purpose: both arms of that comparison drew their
 * probabilities from the same win model, so an arm that optimises *both* sides moves the
 * counterfactual fixture toward parity and must lose winner accuracy whether or not its
 * XIs are better. It was replaced by these — measured on the folds, never on the locked
 * window.
 */
const EvaluationSelectionMetrics: React.FC<{ report: EvaluationFormatReport }> = ({ report }) => {
  const { summary } = report.walk_forward;
  const delta = statMean(summary.specific_vs_typical_delta);

  return (
    <>
      <Typography variant="subtitle1" fontWeight={600} sx={{ mb: 1 }}>
        Selection
      </Typography>
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Metric
          label="Specific XI beyond typical XI"
          value={formatStat(summary.specific_vs_typical_delta)}
          caption={
            'AUC of the eleven that played minus the AUC of the side’s typical eleven. ' +
            'Above zero means the model reads the XI, not just the badge.'
          }
        />
        <Metric
          label="Swap monotonicity"
          value={formatShare(summary.swap_violation_share)}
          caption="Share of one-player upgrades that lower P(win). The gate is under 2% (H-4)."
        />
        <Grid item xs={12} md={4}>
          <Paper variant="outlined" sx={{ p: 2, height: '100%', borderStyle: 'dashed' }}>
            <Typography variant="overline" color="text.secondary">
              Natural experiment (E5)
            </Typography>
            <Typography variant="h5" sx={{ my: 0.5 }} color="text.disabled">
              not measured
            </Typography>
            <Typography variant="caption" color="text.secondary">
              For consecutive matches of one side with 1–3 lineup changes, does Δobjective agree
              with Δoutcome more often than chance? Scheduled for P-7.
            </Typography>
          </Paper>
        </Grid>
      </Grid>
      {delta != null && delta <= 0 && (
        <Alert severity="warning" sx={{ mb: 3 }}>
          The specific XI does not beat the side’s typical XI in this format, so selecting on this
          objective is not supported by the folds.
        </Alert>
      )}
      <Stack sx={{ mb: 3 }}>
        <Typography variant="caption" color="text.secondary">
          Selection-comparison winner accuracy is not reported: it cannot separate a better XI from
          a fixture moved toward parity, which is why P-0’s gate was replaced by these.
        </Typography>
      </Stack>
    </>
  );
};

export default EvaluationSelectionMetrics;
