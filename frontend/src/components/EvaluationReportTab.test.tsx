import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import EvaluationReportTab from './EvaluationReportTab';
import type {
  EvaluationE5,
  EvaluationFormatReport,
  EvaluationGate,
  EvaluationReport,
} from '../types';

const mockEvaluationReport = vi.fn();
vi.mock('../api', () => ({
  api: {
    evaluationReport: (...args: unknown[]) => mockEvaluationReport(...args),
  },
}));

function formatReport(overrides: Partial<EvaluationFormatReport> = {}): EvaluationFormatReport {
  return {
    n_matches: 4210,
    walk_forward: {
      folds: [
        {
          cutoff: '2024-01-01',
          end: '2024-04-01',
          n_train: 3000,
          n_eval: 210,
          objective_auc: 0.72,
          display_auc_mean: 0.747,
          display_brier_mean: 0.22,
          base_rate_brier: 0.25,
          swap_monotonicity: { upgrades: 250, violations: 1, violation_share: 0.004 },
          display_swap_monotonicity: { upgrades: 250, violations: 12, violation_share: 0.048 },
          specific_vs_typical: {
            n: 210,
            auc_specific_xi: 0.74,
            auc_typical_xi: 0.695,
            delta: 0.045,
          },
        },
      ],
      summary: {
        objective_auc: { mean: 0.72, sd: 0.01, n_folds: 7 },
        display_auc: { mean: 0.747, sd: 0.012, n_folds: 7 },
        base_rate_brier: { mean: 0.25, sd: 0.002, n_folds: 7 },
        swap_violation_share: { mean: 0.003, sd: 0.001, n_folds: 7 },
        display_swap_violation_share: { mean: 0.048, sd: 0.006, n_folds: 7 },
        specific_vs_typical_delta: { mean: 0.045, sd: 0.021, n_folds: 7 },
        performance: {
          targets: {
            runs: {
              headline: true,
              model: {
                within_match_spearman: { mean: 0.33, sd: 0.01, n_folds: 7 },
                pinball: { mean: 2.93, sd: 0.1, n_folds: 7 },
                interval: {
                  coverage_80: { mean: 0.897, sd: 0.01, n_folds: 7 },
                  width_80: { mean: 29.1, sd: 1.2, n_folds: 7 },
                },
              },
              career_mean: {
                within_match_spearman: { mean: 0.29, sd: 0.01, n_folds: 7 },
                pinball: { mean: 5.09, sd: 0.2, n_folds: 7 },
              },
            },
          },
        },
        simulation: {
          win: {
            brier: {
              display: { mean: 0.21, sd: 0.01, n_folds: 7 },
              simulated: { mean: 0.213, sd: 0.01, n_folds: 7 },
            },
          },
          totals: {
            first_innings: {
              coverage_80: { mean: 0.764, sd: 0.02, n_folds: 7 },
              width_80: { mean: 82.8, sd: 3, n_folds: 7 },
            },
          },
        },
      },
    },
    locked: {
      cutoff: '2026-09-02',
      end: '9999-12-31',
      n_train: 5000,
      n_eval: 332,
      objective_auc: 0.723,
      display_auc_mean: 0.751,
      note: 'locked window (H-19): scored once per release, never used for a choice',
      recalibrated_targets: [],
      performance: {
        targets: {
          runs: {
            model: {
              within_match_spearman: 0.317,
              interval: { coverage_80: 0.897, width_80: 29.1 },
            },
          },
        },
      },
      simulation: {
        totals: { first_innings: { coverage_80: 0.786, width_80: 90.6, dispersion_ratio: 0.98 } },
        win: { brier: { display: 0.2, simulated: 0.203 } },
      },
    },
    simulation_decision: {
      simulated_win_probability_within_tolerance: true,
      reason: 'simulated P(win) within tolerance of the display model on the folds: a probability',
      shared_factor: true,
      chase_orientation: 'chasing',
      served: false,
    },
    e5_lineup_only: e5Report(),
    selection_decision: e5Report().decision,
    ...overrides,
  };
}

function e5Report(): EvaluationE5 {
  const development = {
    pairs_scored: 1204,
    agreed: 613,
    agreement: 0.509,
    standard_error: 0.014,
    ci95: [0.481, 0.537] as [number, number],
    effect_size: { n: 2600, median_abs: 0.021, p90_abs: 0.065 },
    derived_bar: { n_pairs: 2600, expected_if_exactly_right: 0.523, bar: 0.501 },
    passes_derived_bar: true,
  };
  return {
    definition: 'both elevens scored in the later fixture at its as-of',
    why_not_as_played: '§5’s as-played form is not computed: it scores mean reversion',
    pairs: { total: 2600, development: 2600, locked: 400 },
    walk_forward: {
      folds: [
        { cutoff: '2024-01-01', end: '2024-04-01', pairs: 300, pairs_scored: 140, agreement: 0.51 },
      ],
      summary: { agreement: { mean: 0.51, sd: 0.02, n_folds: 7 } },
    },
    development,
    locked: { pairs_scored: 190, agreement: 0.52, standard_error: 0.036 },
    decision: {
      agreement: 0.509,
      pairs_scored: 1204,
      standard_error: 0.014,
      bar: 0.501,
      expected_if_exactly_right: 0.523,
      passes_derived_bar: true,
      optimised_selection_served: true,
      reason:
        'optimised selection served in T20, because E5 lineup-only agreement 0.509 over 1204 pairs against the derived bar 0.501 (an exactly-right objective would score 0.523): passes',
    },
  };
}

const gateRegistry: Record<string, EvaluationGate> = {
  E5: {
    id: 'E5',
    name: 'Natural experiment, lineup-only',
    varies: 'the eleven',
    fixed: 'the opponent eleven and the as-of',
    decides: 'sign agreement at or above the derived bar',
  },
  'H-4': {
    id: 'H-4',
    name: 'Swap monotonicity',
    varies: 'one player’s ratings',
    fixed: 'the other ten',
    decides: 'violation share under 2%',
  },
};

function report(overrides: Partial<EvaluationReport> = {}): EvaluationReport {
  return {
    generated_at: '2026-08-30T12:00:00+00:00',
    source: 'PostgresSource',
    cutoffs: ['2024-01-01'],
    locked_start: '2026-09-02',
    locked_window: {
      start: '2026-09-02',
      rotated_on: '2026-09-02',
      previous_start: '2025-09-01',
      reason: 'the migration read the previous window',
      retired_into_folds: ['2025-09-01', '2025-12-01'],
    },
    seeds: [0, 1, 2],
    n_rows: 22734,
    n_player_rows: 463818,
    formats: { T20: formatReport() },
    serving_parity: { passed: true },
    gates: { registry: gateRegistry, passed: true, problems: [] },
    ...overrides,
  };
}

describe('EvaluationReportTab', () => {
  beforeEach(() => {
    mockEvaluationReport.mockReset();
  });

  it('renders the walk-forward table with the locked window labelled', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText('Walk-forward')).toBeInTheDocument());
    expect(screen.getByText('2024-01-01 → 2024-04-01')).toBeInTheDocument();
    expect(screen.getByText('2026-09-02 → today')).toBeInTheDocument();
    expect(screen.getByText('locked')).toBeInTheDocument();
  });

  it('says which window a number came from and when the line last moved', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText('locked from 2026-09-02')).toBeInTheDocument());
    expect(screen.getByText('window rotated 2026-09-02, from 2025-09-01')).toBeInTheDocument();
  });

  it('says when the report prints a metric the glossary does not explain', async () => {
    mockEvaluationReport.mockResolvedValue({
      ...report(),
      glossary: {
        entries: {},
        passed: false,
        problems: ["metric 'brand_new_score' is reported with no glossary entry"],
      },
    });
    render(<EvaluationReportTab />);

    expect(await screen.findByText(/Metric glossary incomplete/)).toBeInTheDocument();
    expect(screen.getByText(/brand_new_score/)).toBeInTheDocument();
  });

  it('shows the three selection gates with E5 against its derived bar', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() =>
      expect(screen.getByText(/Specific XI beyond typical XI/)).toBeInTheDocument(),
    );
    expect(screen.getAllByText('0.045 ± 0.021').length).toBeGreaterThan(0);
    expect(screen.getByText('Swap monotonicity')).toBeInTheDocument();
    expect(screen.getByText(/Natural experiment \(E5\), lineup-only/)).toBeInTheDocument();
    expect(screen.getByText('0.509 ± 0.014 (n=1,204)')).toBeInTheDocument();
    // The caption is the harness's own definition and the run's numbers -- the frontend
    // adds no prose of its own about what E5 means (L-1).
    expect(
      screen.getByText(/both elevens scored in the later fixture at its as-of\. Bar 0.501/),
    ).toBeInTheDocument();
    expect(screen.getByText(/would score 0.523\) — passes/)).toBeInTheDocument();
  });

  it('states the display surface’s swap share beside the objective’s, as a measurement', async () => {
    // B-7: the display model is the number a person watches move in the Team Lab, and it
    // violates at 3-7%. The tile says so, and says H-4's 2% line is not its contract.
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() =>
      expect(screen.getByText('Swap monotonicity, display surface')).toBeInTheDocument(),
    );
    expect(screen.getByText('4.8%')).toBeInTheDocument();
    expect(screen.getByText(/H-4’s 2% line is the objective’s contract/)).toBeInTheDocument();
  });

  it('states the selection decision per format with the locked window labelled beside it', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText('Optimised selection: yes.')).toBeInTheDocument());
    expect(screen.getByText(/because E5 lineup-only agreement 0.509/)).toBeInTheDocument();
    expect(
      screen.getByText(
        /Locked window, labelled and never used for the choice: 0.520 ± 0.036 \(n=190\)/,
      ),
    ).toBeInTheDocument();
  });

  it('says optimised selection is off where the policy scopes it off', async () => {
    const scoped = formatReport();
    scoped.selection_decision = {
      ...e5Report().decision,
      optimised_selection_served: false,
      reason: 'optimised selection not served in T20, because E5 lineup-only agreement 0.509 fails',
    };
    mockEvaluationReport.mockResolvedValue(report({ formats: { T20: scoped } }));
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText('Optimised selection: no.')).toBeInTheDocument());
    expect(screen.getByText(/optimised selection not served in T20/)).toBeInTheDocument();
  });

  it('renders each gate’s varies / fixed / decides triple beside its number (H-23)', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getAllByText('Varies:').length).toBeGreaterThan(0));
    expect(screen.getByText(/the opponent eleven and the as-of/)).toBeInTheDocument();
    expect(screen.getByText(/violation share under 2%/)).toBeInTheDocument();
  });

  it('keeps the E5 slot labelled when the format has no scorable pairs', async () => {
    const thin = formatReport();
    thin.e5_lineup_only = {
      ...e5Report(),
      decision: {
        agreement: null,
        bar: null,
        passes_derived_bar: null,
        optimised_selection_served: false,
        reason:
          'optimised selection not served in TEST, because E5 could not be scored (no pairs whose result moved)',
      },
    };
    thin.selection_decision = thin.e5_lineup_only.decision;
    mockEvaluationReport.mockResolvedValue(report({ formats: { TEST: thin } }));
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText('not measured')).toBeInTheDocument());
    expect(screen.getByText(/No pairs whose result moved could be scored/)).toBeInTheDocument();
  });

  it('reports a failed gate registry as an error (H-23)', async () => {
    mockEvaluationReport.mockResolvedValue(
      report({
        gates: {
          registry: gateRegistry,
          passed: false,
          problems: ['gate E5: ODI carries nothing'],
        },
      }),
    );
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText(/Gate registry FAILED/)).toBeInTheDocument());
    expect(screen.getByText(/gate E5: ODI carries nothing/)).toBeInTheDocument();
  });

  it('shows the per-target performance with width beside coverage', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText('Performance, locked window')).toBeInTheDocument());
    expect(screen.getByText('Performance, mean over folds')).toBeInTheDocument();
    expect(screen.getAllByText('10–90 coverage').length).toBeGreaterThan(0);
    expect(screen.getAllByText('10–90 width').length).toBeGreaterThan(0);
    expect(screen.getAllByText('89.7%').length).toBeGreaterThan(0);
  });

  it('shows the E2 decision and the simulated totals', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText('Simulation (E2)')).toBeInTheDocument());
    expect(screen.getByText('simulated P(win) is a probability')).toBeInTheDocument();
    expect(screen.getByText('display model is the headline')).toBeInTheDocument();
    expect(screen.getByText('78.6%')).toBeInTheDocument();
  });

  /**
   * B-12: a fold whose calibration window was too thin to fit the shared match factor ships
   * a different simulator. The tab has to say so where it shows the fold means, or the
   * headline coverage silently averages two simulators.
   */
  it('says how many folds simulated without a shared match factor, and their totals without them', async () => {
    const odi = formatReport();
    // No locked simulation: the table is showing the fold means, which is where the split lives.
    odi.locked.simulation = undefined;
    odi.walk_forward.summary.simulation = {
      ...odi.walk_forward.summary.simulation,
      shared_factor_folds: {
        folds_scored: 10,
        with_shared_factor: 9,
        without_shared_factor: 1,
        windows_without_shared_factor: ['2026-03-01'],
        totals_with_shared_factor: {
          first_innings: {
            coverage_80: { mean: 0.77, sd: 0.02, n_folds: 9 },
            width_80: { mean: 153.9, sd: 6, n_folds: 9 },
          },
        },
      },
    };
    mockEvaluationReport.mockResolvedValue(report({ formats: { ODI: odi } }));
    render(<EvaluationReportTab />);

    await waitFor(() =>
      expect(screen.getByText('1 of 10 folds without a shared factor')).toBeInTheDocument(),
    );
    expect(screen.getByText('First innings, folds with a shared match factor')).toBeInTheDocument();
    expect(screen.getByText('76.4%')).toBeInTheDocument();
    expect(screen.getByText('77.0%')).toBeInTheDocument();
    expect(screen.getByText(/2026-03-01/)).toBeInTheDocument();
  });

  it('reports a failed serving-parity check as an error, not a footnote', async () => {
    mockEvaluationReport.mockResolvedValue(report({ serving_parity: { passed: false } }));
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText(/parity FAILED/)).toBeInTheDocument());
  });

  it('says where the report comes from when there is none', async () => {
    mockEvaluationReport.mockRejectedValue(new Error('no evaluation report'));
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText(/No evaluation report/)).toBeInTheDocument());
    expect(screen.getByText(/make evaluate/)).toBeInTheDocument();
  });

  it('warns when the specific XI does not beat the typical XI', async () => {
    const weak = formatReport();
    weak.walk_forward.summary.specific_vs_typical_delta = { mean: -0.004, sd: 0.02, n_folds: 7 };
    mockEvaluationReport.mockResolvedValue(report({ formats: { TEST: weak } }));
    render(<EvaluationReportTab />);

    await waitFor(() =>
      expect(
        screen.getByText(/does not beat the side’s typical XI in this format/),
      ).toBeInTheDocument(),
    );
  });

  it('says why a format was not simulated instead of showing an empty table', async () => {
    const test = formatReport();
    test.locked.simulation = { skipped_reason: 'format has no innings length; not simulated' };
    mockEvaluationReport.mockResolvedValue(report({ formats: { TEST: test } }));
    render(<EvaluationReportTab />);

    await waitFor(() =>
      expect(screen.getByText(/no innings length; not simulated/)).toBeInTheDocument(),
    );
  });
});
