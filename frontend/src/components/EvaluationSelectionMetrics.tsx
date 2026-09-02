import React from 'react';
import { Alert, Grid, Paper, Stack, Typography } from '@mui/material';
import type { EvaluationFormatReport, EvaluationGate } from '../types';
import { formatShare, formatStat, statMean } from '../utils/evaluationReport';

/**
 * H-23: every gate says, beside its number, what it varies, what it holds fixed and what
 * decides. The triple comes from the report's embedded registry, never from the browser.
 */
const GateTriple: React.FC<{ gate?: EvaluationGate }> = ({ gate }) => {
  if (!gate) return null;
  return (
    <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
      <strong>Varies:</strong> {gate.varies}. <strong>Fixed:</strong> {gate.fixed}.{' '}
      <strong>Decides:</strong> {gate.decides}.
    </Typography>
  );
};

const Metric: React.FC<{
  label: string;
  value: string;
  caption: string;
  gate?: EvaluationGate;
  dashed?: boolean;
  muted?: boolean;
}> = ({ label, value, caption, gate, dashed, muted }) => (
  <Grid item xs={12} md={4}>
    <Paper
      variant="outlined"
      sx={{ p: 2, height: '100%', borderStyle: dashed ? 'dashed' : 'solid' }}
    >
      <Typography variant="overline" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="h5" sx={{ my: 0.5 }} color={muted ? 'text.disabled' : 'text.primary'}>
        {value}
      </Typography>
      <Typography variant="caption" color="text.secondary" component="div">
        {caption}
      </Typography>
      <GateTriple gate={gate} />
    </Paper>
  </Grid>
);

/** "0.509 ± 0.007 (n=5,504)": a rate with its sampling error and denominator. */
function formatAgreement(
  agreement: number | null,
  se: number | null | undefined,
  n: number | undefined,
): string {
  if (agreement == null) return 'not measured';
  const point = agreement.toFixed(3);
  const spread = se == null ? '' : ` ± ${se.toFixed(3)}`;
  const count = n == null ? '' : ` (n=${n.toLocaleString()})`;
  return `${point}${spread}${count}`;
}

/**
 * The three selection gates.
 *
 * P-0's winner accuracy is not here on purpose: both arms of that comparison drew their
 * probabilities from the same win model, so an arm that optimises *both* sides moves the
 * counterfactual fixture toward parity and must lose winner accuracy whether or not its
 * XIs are better. It was replaced by these — measured on the folds, never on the locked
 * window. E5 varies one side only: the eleven, with the opponent and the as-of fixed.
 */
const EvaluationSelectionMetrics: React.FC<{
  report: EvaluationFormatReport;
  gates?: Record<string, EvaluationGate>;
}> = ({ report, gates }) => {
  const { summary } = report.walk_forward;
  const delta = statMean(summary.specific_vs_typical_delta);
  const e5 = report.e5_lineup_only;
  const decision = report.selection_decision;
  const bar = decision?.bar;
  const exactlyRight = decision?.expected_if_exactly_right;
  const e5Caption =
    e5 && decision?.agreement != null
      ? `Sign agreement between the objective’s preference among one side’s consecutive elevens (1–3 changes, ` +
        `both scored in the later fixture at its as-of) and the result change. Bar derived from the objective’s ` +
        `own claimed effect: ${bar?.toFixed(3) ?? '—'} (an exactly-right objective would score ` +
        `${exactlyRight?.toFixed(3) ?? '—'}) — ${decision.passes_derived_bar ? 'passes' : 'fails'}.`
      : e5
        ? `No pairs whose result moved could be scored in this format. ${e5.why_not_as_played}.`
        : 'The harness that wrote this report predates the lineup-only E5 metric; re-run make evaluate.';

  return (
    <>
      <Typography variant="subtitle1" fontWeight={600} sx={{ mb: 1 }}>
        Selection
      </Typography>
      <Grid container spacing={2} sx={{ mb: 2 }}>
        <Metric
          label="Specific XI beyond typical XI"
          value={formatStat(summary.specific_vs_typical_delta)}
          caption={
            'AUC of the eleven that played minus the AUC of the side’s typical eleven. ' +
            'Above zero means the model reads the XI, not just the badge.'
          }
          gate={gates?.['specific-vs-typical']}
        />
        <Metric
          label="Swap monotonicity"
          value={formatShare(summary.swap_violation_share)}
          caption="Share of one-player upgrades that lower P(win). The gate is under 2% (H-4)."
          gate={gates?.['H-4']}
        />
        <Metric
          label="Natural experiment (E5), lineup-only"
          value={formatAgreement(
            decision?.agreement ?? null,
            decision?.standard_error,
            decision?.pairs_scored,
          )}
          caption={e5Caption}
          gate={gates?.['E5']}
          dashed={decision?.agreement == null}
          muted={decision?.agreement == null}
        />
      </Grid>
      {decision && (
        <Alert severity={decision.optimised_selection_served ? 'success' : 'info'} sx={{ mb: 3 }}>
          <strong>
            Optimised selection: {decision.optimised_selection_served ? 'yes' : 'no'}.
          </strong>{' '}
          {decision.reason}
          {e5?.locked.agreement != null && (
            <>
              {' '}
              Locked window, labelled and never used for the choice:{' '}
              {formatAgreement(
                e5.locked.agreement,
                e5.locked.standard_error,
                e5.locked.pairs_scored,
              )}
              .
            </>
          )}
        </Alert>
      )}
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
