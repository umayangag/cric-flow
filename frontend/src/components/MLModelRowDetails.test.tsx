import { render, screen, cleanup } from '@testing-library/react';
import React from 'react';
import MLModelRowDetails from './MLModelRowDetails';
import type { MLModelStat } from '../types';

function model(overrides: Partial<MLModelStat>): MLModelStat {
  return {
    model_name: 'Batting',
    match_format: 'T20I',
    ...overrides,
  };
}

describe('MLModelRowDetails', () => {
  afterEach(cleanup);

  it('renders tuned parameters as key=value chips', () => {
    render(
      <MLModelRowDetails model={model({ tuned_parameters: { algorithm: 'rf', max_depth: 12 } })} />,
    );

    expect(screen.getByText('algorithm=rf')).toBeDefined();
    expect(screen.getByText('max_depth=12')).toBeDefined();
  });

  it('joins array metrics and skips nested objects that would render as [object Object]', () => {
    render(
      <MLModelRowDetails
        model={model({
          metrics: {
            mae: 6.5,
            cv_fold_scores: [-7.1, -6.9],
            target_context: { target_mean: 20 },
          },
        })}
      />,
    );

    expect(screen.getByText('mae=6.5')).toBeDefined();
    expect(screen.getByText('cv_fold_scores=-7.1, -6.9')).toBeDefined();
    expect(screen.queryByText(/target_context/)).toBeNull();
  });

  it('shows the dataset shape line when the model reports its sample count', () => {
    render(
      <MLModelRowDetails
        model={model({ n_samples: 1000, n_features: 30, validation_method: 'trailing_holdout' })}
      />,
    );

    expect(screen.getByText(/n_samples: 1000/)).toBeDefined();
    expect(screen.getByText(/n_features: 30/)).toBeDefined();
    expect(screen.getByText(/validation: trailing_holdout/)).toBeDefined();
  });

  it('renders nothing at all for a model with no tuning, metrics or audit', () => {
    const { container } = render(<MLModelRowDetails model={model({})} />);

    expect(container.textContent).toBe('');
  });
});

describe('MLModelRowDetails tuning insights', () => {
  afterEach(cleanup);

  it('surfaces the headline tuning numbers next to their explanations', () => {
    const { container } = render(
      <MLModelRowDetails
        model={model({
          metrics: { mae: 6.5, baseline_improvement_pct: 34.2, val_still_improving: true },
        })}
      />,
    );

    expect(screen.getByText('Tuning insights')).toBeDefined();
    expect(container.textContent).toContain('Baseline improvement:34.2%');
    // A boolean insight is rendered as Yes/No, not as "true".
    expect(container.textContent).toContain('Val still improving:Yes');
    expect(container.textContent).not.toContain('Val still improving:true');
  });

  // The panel is omitted rather than rendered empty when a model carries only raw metrics.
  it('is omitted when no metric qualifies as an insight', () => {
    render(<MLModelRowDetails model={model({ metrics: { mae: 6.5 } })} />);

    expect(screen.getByText('mae=6.5')).toBeDefined();
    expect(screen.queryByText('Tuning insights')).toBeNull();
  });
});

describe('MLModelRowDetails MLQA audit', () => {
  afterEach(cleanup);

  const audit = {
    audit_status: 'WARNING' as const,
    key_findings: ['High Overfitting Risk'],
    bias_report: 'No protected groups defined.',
    final_verdict: 'REVIEW',
    checks: {
      overfitting: { delta: 0.5, relative_delta: 0.123, threshold: 0.1, flagged: true },
      stability: { cv_std: 0.2, relative_cv_std: 0.05, threshold: 0.08, flagged: false },
    },
  };

  it('renders the verdict, findings and per-check chips', () => {
    render(<MLModelRowDetails model={model({ mlqa_audit: audit })} />);

    expect(screen.getByText('WARNING')).toBeDefined();
    expect(screen.getByText('REVIEW')).toBeDefined();
    expect(screen.getByText('High Overfitting Risk')).toBeDefined();
    expect(screen.getByText('Δ=0.5 (12.3%) ⚠')).toBeDefined();
    expect(screen.getByText('σ=0.2 (5.0%) ✓')).toBeDefined();
  });

  // The backend returns Infinity when the reference score is near zero. "Infinity%" would
  // read as a measurement; "n/a" says the ratio could not be computed.
  it('shows n/a rather than Infinity when the relative metric is undefined', () => {
    render(
      <MLModelRowDetails
        model={model({
          mlqa_audit: {
            ...audit,
            checks: {
              overfitting: { delta: 0.5, relative_delta: Infinity, threshold: 0.1, flagged: true },
            },
          },
        })}
      />,
    );

    expect(screen.getByText('Δ=0.5 (n/a) ⚠')).toBeDefined();
  });

  it('ranks feature importance by weight and keeps the list to the top 15', () => {
    const featureImportance = Object.fromEntries(
      Array.from({ length: 20 }, (_, i) => [`feature_${i}`, (i + 1) / 100]),
    );

    render(
      <MLModelRowDetails
        model={model({ mlqa_audit: audit, feature_importance: featureImportance })}
      />,
    );

    // feature_19 is the heaviest at 0.20; feature_4 is the 15th and last shown.
    expect(screen.getByText('feature_19: 20.0%')).toBeDefined();
    expect(screen.getByText('feature_5: 6.0%')).toBeDefined();
    expect(screen.queryByText('feature_4: 5.0%')).toBeNull();
  });
});
