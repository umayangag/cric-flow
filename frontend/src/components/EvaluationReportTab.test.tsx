import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import EvaluationReportTab from './EvaluationReportTab';
import type { EvaluationFormatReport, EvaluationReport } from '../types';

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
      cutoff: '2025-09-01',
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
    ...overrides,
  };
}

function report(overrides: Partial<EvaluationReport> = {}): EvaluationReport {
  return {
    generated_at: '2026-08-30T12:00:00+00:00',
    source: 'PostgresSource',
    cutoffs: ['2024-01-01'],
    locked_start: '2025-09-01',
    seeds: [0, 1, 2],
    n_rows: 22734,
    n_player_rows: 463818,
    formats: { T20: formatReport() },
    serving_parity: { passed: true },
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
    expect(screen.getByText('2025-09-01 → today')).toBeInTheDocument();
    expect(screen.getByText('locked')).toBeInTheDocument();
  });

  it('shows the two selection metrics and a labelled slot for the third', async () => {
    mockEvaluationReport.mockResolvedValue(report());
    render(<EvaluationReportTab />);

    await waitFor(() =>
      expect(screen.getByText(/Specific XI beyond typical XI/)).toBeInTheDocument(),
    );
    expect(screen.getAllByText('0.045 ± 0.021').length).toBeGreaterThan(0);
    expect(screen.getByText(/Swap monotonicity/)).toBeInTheDocument();
    expect(screen.getByText(/Natural experiment \(E5\)/)).toBeInTheDocument();
    expect(screen.getByText('not measured')).toBeInTheDocument();
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

  it('reports a failed serving-parity check as an error, not a footnote', async () => {
    mockEvaluationReport.mockResolvedValue(report({ serving_parity: { passed: false } }));
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText(/parity FAILED/)).toBeInTheDocument());
  });

  it('says where the report comes from when there is none', async () => {
    mockEvaluationReport.mockRejectedValue(new Error('no evaluation report'));
    render(<EvaluationReportTab />);

    await waitFor(() => expect(screen.getByText(/No evaluation report/)).toBeInTheDocument());
    expect(screen.getByText(/make xi-evaluate/)).toBeInTheDocument();
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
