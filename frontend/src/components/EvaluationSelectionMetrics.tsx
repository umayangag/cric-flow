import React from 'react';
import { Alert, Grid, Paper, Stack, Typography } from '@mui/material';
import type { EvaluationFormatReport, EvaluationGate } from '../types';
import { MetricInfo } from './common/MetricInfo';
import MetricValue from './common/MetricValue';
import { formatShare, formatStat, statMean, type ReportNumber } from '../utils/evaluationReport';

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
  /** The glossary key this tile's number is reported under (L-1). */
  metricKey: string;
  /** The number as this tile writes it, for the popover and for the reader. */
  value: string;
  /** The same number unformatted, so the tile can be painted against its band. */
  measured: ReportNumber;
  caption: string;
  gate?: EvaluationGate;
  dashed?: boolean;
  muted?: boolean;
}> = ({ label, metricKey, value, measured, caption, gate, dashed, muted }) => (
  <Grid item xs={12} md={4}>
    <Paper
      variant="outlined"
      sx={{ p: 2, height: '100%', borderStyle: dashed ? 'dashed' : 'solid' }}
    >
      <Typography variant="overline" color="text.secondary">
        {label}
        <MetricInfo metricKey={metricKey} label={label} value={value} />
      </Typography>
      <Typography variant="h5" sx={{ my: 0.5 }} color={muted ? 'text.disabled' : 'text.primary'}>
        <MetricValue metricKey={metricKey} value={measured} text={value} variant="text" />
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
 * The three selection gates, and the display surface's swap share beside them.
 *
 * P-0's winner accuracy is not here on purpose: both arms of that comparison drew their
 * probabilities from the same win model, so an arm that optimises *both* sides moves the
 * counterfactual fixture toward parity and must lose winner accuracy whether or not its
 * XIs are better. It was replaced by these — measured on the folds, never on the locked
 * window. E5 varies one side only: the eleven, with the opponent and the as-of fixed.
 *
 * The fourth tile is a measurement, not a gate (B-7): the display model is the surface a
 * person watches move in the Team Lab, and until it was measured nobody could say whether
 * it agreed with itself about what a better player is. It sits beside H-4's tile because
 * the probe is the same one, and carries no gate triple because nothing decides on it.
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
  // The caption is the harness's own definition and the run's numbers; what the metric
  // *means* is the glossary's, one click away (L-1), and is not restated here.
  const e5Caption =
    e5 && decision?.agreement != null
      ? `${e5.definition}. Bar ${bar?.toFixed(3) ?? '—'} (an exactly-right objective would score ` +
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
          metricKey="specific_vs_typical_delta"
          value={formatStat(summary.specific_vs_typical_delta)}
          measured={summary.specific_vs_typical_delta}
          caption="Measured on the folds, against the side’s own typical eleven on the same fixtures."
          gate={gates?.['specific-vs-typical']}
        />
        <Metric
          label="Swap monotonicity"
          metricKey="swap_violation_share"
          value={formatShare(summary.swap_violation_share)}
          measured={summary.swap_violation_share}
          caption="One player upgraded at a time, the other ten and the opponent held fixed (H-4)."
          gate={gates?.['H-4']}
        />
        <Metric
          label="Swap monotonicity, display surface"
          metricKey="display_swap_violation_share"
          value={formatShare(summary.display_swap_violation_share)}
          measured={summary.display_swap_violation_share}
          caption="The same upgrade scored on the model a person watches. H-4’s 2% line is the objective’s contract, not this surface’s; this is measured and stated, and decides nothing (B-7)."
        />
        <Metric
          label="Natural experiment (E5), lineup-only"
          metricKey="agreement"
          value={formatAgreement(
            decision?.agreement ?? null,
            decision?.standard_error,
            decision?.pairs_scored,
          )}
          measured={decision?.agreement}
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
